// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"fmt"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

// TestAccCustomObjectTypeResource_basic exercises Create/Read/Update/Delete
// against a live NetBox instance. It requires TF_ACC=1, NETBOX_URL and
// NETBOX_TOKEN, and the netbox-custom-objects plugin to be installed on
// the target server.
func TestAccCustomObjectTypeResource_basic(t *testing.T) {
	// A time-based suffix keeps parallel runs from colliding.
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	name := fmt.Sprintf("tfacc_%s", suffix)
	slug := fmt.Sprintf("tfacc-%s", suffix)

	resourceAddr := "netbox_custom_object_type.test"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheckNetbox(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccCustomObjectTypeResourceConfig(name, slug, "First"),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(resourceAddr,
						tfjsonpath.New("name"), knownvalue.StringExact(name)),
					statecheck.ExpectKnownValue(resourceAddr,
						tfjsonpath.New("slug"), knownvalue.StringExact(slug)),
					statecheck.ExpectKnownValue(resourceAddr,
						tfjsonpath.New("description"), knownvalue.StringExact("First")),
				},
			},
			{
				ResourceName:      resourceAddr,
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				Config: testAccCustomObjectTypeResourceConfig(name, slug, "Second"),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(resourceAddr,
						tfjsonpath.New("description"), knownvalue.StringExact("Second")),
				},
			},
		},
	})
}

func testAccCustomObjectTypeResourceConfig(name, slug, description string) string {
	return fmt.Sprintf(`
resource "netbox_custom_object_type" "test" {
  description = %[3]q
  name        = %[1]q
  slug        = %[2]q
}
`, name, slug, description)
}
