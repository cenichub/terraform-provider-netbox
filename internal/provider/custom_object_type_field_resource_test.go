// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

// TestAccCustomObjectTypeFieldResource_basic exercises Create/Read/Update/
// Delete against a live NetBox instance. It requires TF_ACC=1, NETBOX_URL
// and NETBOX_TOKEN, and the netbox-custom-objects plugin to be installed
// on the target server. The parent Custom Object Type is created inline
// so the test is self-contained.
func TestAccCustomObjectTypeFieldResource_basic(t *testing.T) {
	// A time-based suffix keeps parallel runs from colliding.
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	typeName := fmt.Sprintf("tfacc_%s", suffix)
	typeSlug := fmt.Sprintf("tfacc-%s", suffix)
	fieldName := "label"

	resourceAddr := "netbox_custom_object_type_field.test"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheckNetbox(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccCustomObjectTypeFieldResourceConfig(
					typeName, typeSlug, fieldName, "First label", true),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(resourceAddr,
						tfjsonpath.New("name"), knownvalue.StringExact(fieldName)),
					statecheck.ExpectKnownValue(resourceAddr,
						tfjsonpath.New("type"), knownvalue.StringExact("text")),
					statecheck.ExpectKnownValue(resourceAddr,
						tfjsonpath.New("label"), knownvalue.StringExact("First label")),
					statecheck.ExpectKnownValue(resourceAddr,
						tfjsonpath.New("required"), knownvalue.Bool(true)),
				},
			},
			{
				ResourceName:      resourceAddr,
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				Config: testAccCustomObjectTypeFieldResourceConfig(
					typeName, typeSlug, fieldName, "Updated label", false),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(resourceAddr,
						tfjsonpath.New("label"), knownvalue.StringExact("Updated label")),
					statecheck.ExpectKnownValue(resourceAddr,
						tfjsonpath.New("required"), knownvalue.Bool(false)),
				},
			},
		},
	})
}

func testAccCustomObjectTypeFieldResourceConfig(typeName, typeSlug, fieldName, label string, required bool) string {
	return fmt.Sprintf(`
resource "netbox_custom_object_type" "test" {
  name = %[1]q
  slug = %[2]q
}

resource "netbox_custom_object_type_field" "test" {
  custom_object_type_id = netbox_custom_object_type.test.id
  label                 = %[4]q
  name                  = %[3]q
  required              = %[5]t
  type                  = "text"
}
`, typeName, typeSlug, fieldName, label, required)
}

// The following unit tests exercise the pure helpers in the resource so
// the package's non-acceptance suite has meaningful coverage of the
// tricky bits (JSON normalization, dotted-name parsing, pointer
// helpers) without requiring TF_ACC or a live NetBox.

func TestSplitDottedObjectType(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in            string
		wantApp       string
		wantModel     string
		wantErrSubstr string
	}{
		{in: "dcim.device", wantApp: "dcim", wantModel: "device"},
		{in: "tenancy.tenant", wantApp: "tenancy", wantModel: "tenant"},
		{in: "custom_objects.thing", wantApp: "custom_objects", wantModel: "thing"},
		{in: "invalid", wantErrSubstr: "app_label"},
		{in: "Too.Uppercase", wantErrSubstr: "app_label"},
		{in: "dcim.", wantErrSubstr: "app_label"},
		{in: ".device", wantErrSubstr: "app_label"},
		{in: "", wantErrSubstr: "app_label"},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			app, model, err := splitDottedObjectType(tc.in)
			if tc.wantErrSubstr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tc.wantErrSubstr)
				}
				if !strings.Contains(err.Error(), tc.wantErrSubstr) {
					t.Fatalf("expected error containing %q, got %q", tc.wantErrSubstr, err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %s", err)
			}
			if app != tc.wantApp || model != tc.wantModel {
				t.Fatalf("got (%q, %q); want (%q, %q)", app, model, tc.wantApp, tc.wantModel)
			}
		})
	}
}

func TestJSONRawFromString(t *testing.T) {
	t.Parallel()

	if _, ok := jsonRawFromString(types.StringNull()); ok {
		t.Fatalf("null string must not produce raw JSON")
	}
	if _, ok := jsonRawFromString(types.StringUnknown()); ok {
		t.Fatalf("unknown string must not produce raw JSON")
	}
	if _, ok := jsonRawFromString(types.StringValue("   ")); ok {
		t.Fatalf("whitespace-only string must not produce raw JSON")
	}
	raw, ok := jsonRawFromString(types.StringValue(`{"a":1}`))
	if !ok {
		t.Fatalf("expected raw JSON to be produced")
	}
	if string(raw) != `{"a":1}` {
		t.Fatalf("unexpected raw JSON: %s", string(raw))
	}
}

func TestStringFromRawJSON(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		raw      json.RawMessage
		wantNull bool
		want     string
	}{
		{name: "nil", raw: nil, wantNull: true},
		{name: "empty", raw: json.RawMessage(""), wantNull: true},
		{name: "explicit_null", raw: json.RawMessage("null"), wantNull: true},
		{name: "number", raw: json.RawMessage("42"), want: "42"},
		{name: "string", raw: json.RawMessage(`"foo"`), want: `"foo"`},
		{name: "object_with_whitespace", raw: json.RawMessage(`{ "a" : 1 }`), want: `{"a":1}`},
		{name: "malformed_preserved", raw: json.RawMessage(`{oops}`), want: `{oops}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := stringFromRawJSON(tc.raw)
			if tc.wantNull {
				if !got.IsNull() {
					t.Fatalf("expected null, got %q", got.ValueString())
				}
				return
			}
			if got.IsNull() {
				t.Fatalf("expected value %q, got null", tc.want)
			}
			if got.ValueString() != tc.want {
				t.Fatalf("got %q, want %q", got.ValueString(), tc.want)
			}
		})
	}
}

func TestBoolPtrAndInt64Ptr(t *testing.T) {
	t.Parallel()

	if boolPtr(types.BoolNull()) != nil {
		t.Fatalf("null bool must map to nil")
	}
	if boolPtr(types.BoolUnknown()) != nil {
		t.Fatalf("unknown bool must map to nil")
	}
	if p := boolPtr(types.BoolValue(true)); p == nil || !*p {
		t.Fatalf("true bool must map to *true")
	}
	if int64Ptr(types.Int64Null()) != nil {
		t.Fatalf("null int64 must map to nil")
	}
	if int64Ptr(types.Int64Unknown()) != nil {
		t.Fatalf("unknown int64 must map to nil")
	}
	if p := int64Ptr(types.Int64Value(7)); p == nil || *p != 7 {
		t.Fatalf("int64 value must map to *7")
	}

	b := true
	if got := boolFromPtr(&b); got.IsNull() || !got.ValueBool() {
		t.Fatalf("*true must map to true")
	}
	if got := boolFromPtr(nil); !got.IsNull() {
		t.Fatalf("nil bool ptr must map to null")
	}
	i := int64(9)
	if got := int64FromPtr(&i); got.IsNull() || got.ValueInt64() != 9 {
		t.Fatalf("*9 must map to 9")
	}
	if got := int64FromPtr(nil); !got.IsNull() {
		t.Fatalf("nil int64 ptr must map to null")
	}
}

func TestJSONStringValidator(t *testing.T) {
	t.Parallel()

	v := jsonStringValidator{}
	ctx := context.Background()

	// null / unknown / empty pass silently.
	for _, val := range []types.String{
		types.StringNull(),
		types.StringUnknown(),
		types.StringValue("   "),
	} {
		resp := &validator.StringResponse{}
		v.ValidateString(ctx, validator.StringRequest{
			ConfigValue: val,
			Path:        path.Root("default"),
		}, resp)
		if resp.Diagnostics.HasError() {
			t.Fatalf("expected no error for %#v, got %s", val, resp.Diagnostics)
		}
	}

	// Valid JSON passes.
	resp := &validator.StringResponse{}
	v.ValidateString(ctx, validator.StringRequest{
		ConfigValue: types.StringValue(`{"a":1}`),
		Path:        path.Root("default"),
	}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("expected no error, got %s", resp.Diagnostics)
	}

	// Invalid JSON emits an attribute error.
	resp = &validator.StringResponse{}
	v.ValidateString(ctx, validator.StringRequest{
		ConfigValue: types.StringValue(`{oops}`),
		Path:        path.Root("default"),
	}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatalf("expected diagnostic error for invalid JSON, got none")
	}
}

func TestNormalizeJSONStringPlanModifier(t *testing.T) {
	t.Parallel()

	m := normalizeJSONStringPlanModifier{}
	ctx := context.Background()

	// null / unknown / empty are left unchanged.
	for _, val := range []types.String{
		types.StringNull(),
		types.StringUnknown(),
		types.StringValue("   "),
	} {
		resp := &planmodifier.StringResponse{PlanValue: val}
		m.PlanModifyString(ctx, planmodifier.StringRequest{PlanValue: val}, resp)
		if !resp.PlanValue.Equal(val) {
			t.Fatalf("expected %#v to be unchanged, got %#v", val, resp.PlanValue)
		}
	}

	// Whitespace-heavy JSON is compacted.
	in := types.StringValue(`  { "a" : [1, 2, 3] }  `)
	resp := &planmodifier.StringResponse{PlanValue: in}
	m.PlanModifyString(ctx, planmodifier.StringRequest{PlanValue: in}, resp)
	if got := resp.PlanValue.ValueString(); got != `{"a":[1,2,3]}` {
		t.Fatalf("expected compact JSON, got %q", got)
	}

	// Invalid JSON is left unchanged (validator surfaces the error).
	bad := types.StringValue(`{oops}`)
	resp = &planmodifier.StringResponse{PlanValue: bad}
	m.PlanModifyString(ctx, planmodifier.StringRequest{PlanValue: bad}, resp)
	if !resp.PlanValue.Equal(bad) {
		t.Fatalf("expected invalid JSON to be unchanged, got %q", resp.PlanValue.ValueString())
	}
}
