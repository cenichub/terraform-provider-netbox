package provider

import (
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/echoprovider"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// testAccProtoV6ProviderFactories is used to instantiate a provider during acceptance testing.
// The factory function is called for each Terraform CLI command to create a provider
// server that the CLI can connect to and interact with.
var testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"netbox": providerserver.NewProtocol6WithError(New("test")()),
}

// testAccProtoV6ProviderFactoriesWithEcho includes the echo provider alongside the netbox provider.
// It allows for testing assertions on data returned by an ephemeral resource during Open.
// The echoprovider is used to arrange tests by echoing ephemeral data into the Terraform state.
// This lets the data be referenced in test assertions with state checks.
var testAccProtoV6ProviderFactoriesWithEcho = map[string]func() (tfprotov6.ProviderServer, error){
	"netbox": providerserver.NewProtocol6WithError(New("test")()),
	"echo":   echoprovider.NewProviderServer(),
}

func testAccPreCheck(t *testing.T) {
	// You can add code here to run prior to any test case execution, for example assertions
	// about the appropriate environment variables being set are common to see in a pre-check
	// function.
}

// testAccPreCheckNetbox skips a test when the NetBox connection env vars are
// not set. Used by tests that talk to a real NetBox instance.
func testAccPreCheckNetbox(t *testing.T) {
	t.Helper()
	if os.Getenv(envNetboxURL) == "" {
		t.Skipf("%s must be set for this acceptance test", envNetboxURL)
	}
	if os.Getenv(envNetboxToken) == "" {
		t.Skipf("%s must be set for this acceptance test", envNetboxToken)
	}
}

// TestAccProvider_ConnectsToNetbox is a live acceptance test that proves the
// provider can successfully Configure against a real NetBox server. Provider
// Configure runs NewClient() + Ping() against /api/status/, so a successful
// plan of an empty config means the URL and token authenticated correctly.
//
// Requires: TF_ACC=1, NETBOX_URL, NETBOX_TOKEN.
func TestAccProvider_ConnectsToNetbox(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheckNetbox(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				// A bare `provider "netbox" {}` block forces Terraform to
				// instantiate and Configure the provider during plan, which
				// exercises the URL + token round-trip against /api/status/.
				// The provider reads url/token from NETBOX_URL/NETBOX_TOKEN.
				Config: `
					provider "netbox" {}

					data "netbox_example" "connect" {}
				`,
			},
		},
	})
}
