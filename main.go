package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	openapi "github.com/alibabacloud-go/darabonba-openapi/v2/client"
	esa "github.com/alibabacloud-go/esa-20240910/v2/client"
	util "github.com/alibabacloud-go/tea-utils/v2/service"
	"github.com/alibabacloud-go/tea/tea"
	"github.com/aliyun/credentials-go/credentials"
	"github.com/pkg/errors"

	extapi "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/cert-manager/cert-manager/pkg/acme/webhook/apis/acme/v1alpha1"
	"github.com/cert-manager/cert-manager/pkg/acme/webhook/cmd"
	cmmetav1 "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	dnsutil "github.com/cert-manager/cert-manager/pkg/issuer/acme/dns/util"
)

var GroupName = os.Getenv("GROUP_NAME")

func main() {
	if GroupName == "" {
		panic("GROUP_NAME must be specified")
	}

	// This will register our custom DNS provider with the webhook serving
	// library, making it available as an API under the provided GroupName.
	// You can register multiple DNS provider implementations with a single
	// webhook, where the Name() method will be used to disambiguate between
	// the different implementations.
	cmd.RunWebhookServer(GroupName,
		&alicloudESAProviderSolver{},
	)
}

// customDNSProviderSolver implements the provider-specific logic needed to
// 'present' an ACME challenge TXT record for Alibaba Cloud ESA.
// To do so, it must implement the `github.com/cert-manager/cert-manager/pkg/acme/webhook.Solver`
// interface.
type alicloudESAProviderSolver struct {
	client    *kubernetes.Clientset
	esaClient *esa.Client
}

// alicloudESAProviderConfig is a structure that is used to decode into when
// solving a DNS01 challenge.
// This information is provided by cert-manager, and may be a reference to
// additional configuration that's needed to solve the challenge for this
// particular certificate or issuer.
type alicloudESAProviderConfig struct {
	AccessKeyId     cmmetav1.SecretKeySelector `json:"accessKeyIdSecretRef"`
	AccessKeySecret cmmetav1.SecretKeySelector `json:"accessKeySecretSecretRef"`
	RegionId        string                     `json:"regionId"`
}

// Name is used as the name for this DNS solver when referencing it on the ACME
// Issuer resource.
// This should be unique **within the group name**, i.e. you can have two
// solvers configured with the same Name() **so long as they do not co-exist
// within a single webhook deployment**.
// For example, `cloudflare` may be used as the name of a solver.
func (c *alicloudESAProviderSolver) Name() string {
	return "alicloud-esa-solver"
}

// Present is responsible for actually presenting the DNS record with the
// DNS provider.
// This method should tolerate being called multiple times with the same value.
// cert-manager itself will later perform a self check to ensure that the
// solver has correctly configured the DNS provider.
func (c *alicloudESAProviderSolver) Present(ch *v1alpha1.ChallengeRequest) error {
	cfg, err := loadConfig(ch.Config)
	if err != nil {
		return err
	}

	fmt.Printf("Decoded configuration: %v\n", cfg)

	accessKeyId, err := c.loadSecretData(cfg.AccessKeyId, ch.ResourceNamespace)
	if err != nil {
		return err
	}
	accessKeySecret, err := c.loadSecretData(cfg.AccessKeySecret, ch.ResourceNamespace)
	if err != nil {
		return err
	}

	// Initialize ESA client
	config := &credentials.Config{
		Type:            tea.String("access_key"),
		AccessKeyId:     tea.String(string(accessKeyId)),
		AccessKeySecret: tea.String(string(accessKeySecret)),
	}
	credential, err := credentials.NewCredential(config)
	if err != nil {
		return fmt.Errorf("failed to create credential: %v", err)
	}

	c.esaClient, err = esa.NewClient(&openapi.Config{
		Credential: credential,
		RegionId:   tea.String(cfg.RegionId),
		Endpoint:   tea.String(fmt.Sprintf("esa.%s.aliyuncs.com", cfg.RegionId)),
	})
	if err != nil {
		return fmt.Errorf("failed to create ESA client: %v", err)
	}

	// Find the site ID for the domain
	siteId, err := c.getSiteId(ch.ResolvedZone)
	if err != nil {
		return fmt.Errorf("failed to get site ID: %v", err)
	}

	// Create TXT record
	recordName := c.extractRecordName(ch.ResolvedFQDN, ch.ResolvedZone)
	err = c.createTxtRecord(siteId, recordName, ch.Key)
	if err != nil {
		return fmt.Errorf("failed to create TXT record: %v", err)
	}

	return nil
}

// CleanUp should delete the relevant TXT record from the DNS provider console.
// If multiple TXT records exist with the same record name (e.g.
// _acme-challenge.example.com) then **only** the record with the same `key`
// value provided on the ChallengeRequest should be cleaned up.
// This is in order to facilitate multiple DNS validations for the same domain
// concurrently.
func (c *alicloudESAProviderSolver) CleanUp(ch *v1alpha1.ChallengeRequest) error {
	cfg, err := loadConfig(ch.Config)
	if err != nil {
		return err
	}

	// Reinitialize client if needed
	if c.esaClient == nil {
		accessKeyId, err := c.loadSecretData(cfg.AccessKeyId, ch.ResourceNamespace)
		if err != nil {
			return err
		}
		accessKeySecret, err := c.loadSecretData(cfg.AccessKeySecret, ch.ResourceNamespace)
		if err != nil {
			return err
		}

		config := &credentials.Config{
			Type:            tea.String("access_key"),
			AccessKeyId:     tea.String(string(accessKeyId)),
			AccessKeySecret: tea.String(string(accessKeySecret)),
		}
		credential, err := credentials.NewCredential(config)
		if err != nil {
			return fmt.Errorf("failed to create credential: %v", err)
		}

		c.esaClient, err = esa.NewClient(&openapi.Config{
			Credential: credential,
			RegionId:   tea.String(cfg.RegionId),
			Endpoint:   tea.String("esa.ap-southeast-1.aliyuncs.com"),
		})
		if err != nil {
			return fmt.Errorf("failed to create ESA client: %v", err)
		}
	}

	// Find the site ID for the domain
	siteId, err := c.getSiteId(ch.ResolvedZone)
	if err != nil {
		return fmt.Errorf("failed to get site ID: %v", err)
	}

	// Find and delete the TXT record
	recordName := c.extractRecordName(ch.ResolvedFQDN, ch.ResolvedZone)
	err = c.deleteTxtRecord(siteId, recordName, ch.Key)
	if err != nil {
		return fmt.Errorf("failed to delete TXT record: %v", err)
	}

	return nil
}

// Initialize will be called when the webhook first starts.
// This method can be used to instantiate the webhook, i.e. initialising
// connections or warming up caches.
// Typically, the kubeClientConfig parameter is used to build a Kubernetes
// client that can be used to fetch resources from the Kubernetes API, e.g.
// Secret resources containing credentials used to authenticate with DNS
// provider accounts.
// The stopCh can be used to handle early termination of the webhook, in cases
// where a SIGTERM or similar signal is sent to the webhook process.
func (c *alicloudESAProviderSolver) Initialize(kubeClientConfig *rest.Config, stopCh <-chan struct{}) error {
	cl, err := kubernetes.NewForConfig(kubeClientConfig)
	if err != nil {
		return err
	}

	c.client = cl

	return nil
}

// loadConfig is a small helper function that decodes JSON configuration into
// the typed config struct.
func loadConfig(cfgJSON *extapi.JSON) (alicloudESAProviderConfig, error) {
	cfg := alicloudESAProviderConfig{}
	// handle the 'base case' where no configuration has been provided
	if cfgJSON == nil {
		return cfg, nil
	}
	if err := json.Unmarshal(cfgJSON.Raw, &cfg); err != nil {
		return cfg, fmt.Errorf("error decoding solver config: %v", err)
	}

	return cfg, nil
}

// loadSecretData loads secret data from a Kubernetes secret
func (c *alicloudESAProviderSolver) loadSecretData(selector cmmetav1.SecretKeySelector, ns string) ([]byte, error) {
	secret, err := c.client.CoreV1().Secrets(ns).Get(context.TODO(), selector.Name, metav1.GetOptions{})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to load secret %q", ns+"/"+selector.Name)
	}

	if data, ok := secret.Data[selector.Key]; ok {
		return data, nil
	}

	return nil, errors.Errorf("no key %q in secret %q", selector.Key, ns+"/"+selector.Name)
}

// getSiteId finds the site ID for a given domain in ESA
func (c *alicloudESAProviderSolver) getSiteId(domain string) (int64, error) {
	request := &esa.ListSitesRequest{
		PageNumber: tea.Int32(1),
		PageSize:   tea.Int32(100),
	}

	runtime := &util.RuntimeOptions{}

	for {
		response, err := c.esaClient.ListSitesWithOptions(request, runtime)
		if err != nil {
			return 0, fmt.Errorf("failed to list sites: %v", err)
		}

		if response.Body.Sites != nil {
			for _, site := range response.Body.Sites {
				if site.SiteName != nil && *site.SiteName == dnsutil.UnFqdn(domain) {
					if site.SiteId != nil {
						return *site.SiteId, nil
					}
				}
			}
		}

		// Check if there are more pages
		if response.Body.PageNumber != nil && response.Body.PageSize != nil && response.Body.TotalCount != nil {
			if *response.Body.PageNumber**response.Body.PageSize >= *response.Body.TotalCount {
				break
			}
			request.PageNumber = tea.Int32(*response.Body.PageNumber + 1)
		} else {
			break
		}
	}

	return 0, fmt.Errorf("site not found for domain: %s", domain)
}

// extractRecordName extracts the record name from FQDN and domain
// For ESA, we need the full FQDN as record name, not just the prefix like ALIDNS
func (c *alicloudESAProviderSolver) extractRecordName(fqdn, domain string) string {
	// ESA requires full FQDN as record name
	return dnsutil.UnFqdn(fqdn)
}

// createTxtRecord creates a TXT record in ESA
func (c *alicloudESAProviderSolver) createTxtRecord(siteId int64, recordName, value string) error {
	request := &esa.CreateRecordRequest{
		SiteId:     tea.Int64(siteId),
		Type:       tea.String("TXT"),
		RecordName: tea.String(recordName),
		Data: &esa.CreateRecordRequestData{
			Value: tea.String(value),
		},
		Ttl: tea.Int32(300), // 5 minutes TTL for ACME challenges
	}

	runtime := &util.RuntimeOptions{}
	_, err := c.esaClient.CreateRecordWithOptions(request, runtime)
	if err != nil {
		return fmt.Errorf("failed to create TXT record: %v", err)
	}

	return nil
}

// deleteTxtRecord deletes a TXT record from ESA
func (c *alicloudESAProviderSolver) deleteTxtRecord(siteId int64, recordName, value string) error {
	// First, find the record ID
	listRequest := &esa.ListRecordsRequest{
		SiteId:     tea.Int64(siteId),
		Type:       tea.String("TXT"),
		RecordName: tea.String(recordName),
		PageNumber: tea.Int32(1),
		PageSize:   tea.Int32(100),
	}

	runtime := &util.RuntimeOptions{}

	for {
		listResponse, err := c.esaClient.ListRecordsWithOptions(listRequest, runtime)
		if err != nil {
			return fmt.Errorf("failed to list records: %v", err)
		}

		if listResponse.Body.Records != nil {
			for _, record := range listResponse.Body.Records {
				if record.Data != nil && record.Data.Value != nil && *record.Data.Value == value {
					// Found the record, delete it
					deleteRequest := &esa.DeleteRecordRequest{
						RecordId: record.RecordId,
					}

					_, err := c.esaClient.DeleteRecordWithOptions(deleteRequest, runtime)
					if err != nil {
						return fmt.Errorf("failed to delete record: %v", err)
					}
					return nil
				}
			}
		}

		// Check if there are more pages
		if listResponse.Body.PageNumber != nil && listResponse.Body.PageSize != nil && listResponse.Body.TotalCount != nil {
			if *listResponse.Body.PageNumber**listResponse.Body.PageSize >= *listResponse.Body.TotalCount {
				break
			}
			listRequest.PageNumber = tea.Int32(*listResponse.Body.PageNumber + 1)
		} else {
			break
		}
	}

	// Record not found, which is OK for cleanup
	return nil
}
