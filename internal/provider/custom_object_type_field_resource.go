// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// Ensure provider defined types fully satisfy framework interfaces.
var (
	_ resource.Resource                = &CustomObjectTypeFieldResource{}
	_ resource.ResourceWithImportState = &CustomObjectTypeFieldResource{}
	_ resource.ResourceWithConfigure   = &CustomObjectTypeFieldResource{}
)

const customObjectTypeFieldsPath = "/api/plugins/custom-objects/custom-object-type-fields/"

// NewCustomObjectTypeFieldResource returns a factory for the Custom
// Object Type Field resource. It is registered on the provider from
// Resources().
func NewCustomObjectTypeFieldResource() resource.Resource {
	return &CustomObjectTypeFieldResource{}
}

// CustomObjectTypeFieldResource manages a field on a NetBox Custom
// Object Type, provided by the netbox-custom-objects plugin. The
// upstream endpoint is
// /api/plugins/custom-objects/custom-object-type-fields/.
type CustomObjectTypeFieldResource struct {
	client *Client
}

// CustomObjectTypeFieldResourceModel describes the Terraform state for a
// Custom Object Type Field. Attribute names mirror the NetBox
// CustomObjectTypeField / CustomObjectTypeFieldRequest schemas.
type CustomObjectTypeFieldResourceModel struct {
	ChoiceSetID             types.Int64  `tfsdk:"choice_set_id"`
	Comments                types.String `tfsdk:"comments"`
	Context                 types.Bool   `tfsdk:"context"`
	CustomObjectTypeID      types.Int64  `tfsdk:"custom_object_type_id"`
	Default                 types.String `tfsdk:"default"`
	Deprecated              types.Bool   `tfsdk:"deprecated"`
	DeprecatedSince         types.String `tfsdk:"deprecated_since"`
	Description             types.String `tfsdk:"description"`
	FilterLogic             types.String `tfsdk:"filter_logic"`
	GroupName               types.String `tfsdk:"group_name"`
	ID                      types.Int64  `tfsdk:"id"`
	IsCloneable             types.Bool   `tfsdk:"is_cloneable"`
	IsPolymorphic           types.Bool   `tfsdk:"is_polymorphic"`
	Label                   types.String `tfsdk:"label"`
	Name                    types.String `tfsdk:"name"`
	OnDeleteBehavior        types.String `tfsdk:"on_delete_behavior"`
	Primary                 types.Bool   `tfsdk:"primary"`
	RelatedName             types.String `tfsdk:"related_name"`
	RelatedObjectFilter     types.String `tfsdk:"related_object_filter"`
	RelatedObjectType       types.String `tfsdk:"related_object_type"`
	RelatedObjectTypeInput  types.String `tfsdk:"related_object_type_input"`
	RelatedObjectTypes      types.String `tfsdk:"related_object_types"`
	RelatedObjectTypesInput types.Set    `tfsdk:"related_object_types_input"`
	Required                types.Bool   `tfsdk:"required"`
	SchemaID                types.Int64  `tfsdk:"schema_id"`
	ScheduledRemoval        types.String `tfsdk:"scheduled_removal"`
	SearchWeight            types.Int64  `tfsdk:"search_weight"`
	Type                    types.String `tfsdk:"type"`
	UIEditable              types.String `tfsdk:"ui_editable"`
	UIVisible               types.String `tfsdk:"ui_visible"`
	Unique                  types.Bool   `tfsdk:"unique"`
	URL                     types.String `tfsdk:"url"`
	ValidationMaximum       types.Int64  `tfsdk:"validation_maximum"`
	ValidationMinimum       types.Int64  `tfsdk:"validation_minimum"`
	ValidationRegex         types.String `tfsdk:"validation_regex"`
	Weight                  types.Int64  `tfsdk:"weight"`
}

// customObjectTypeFieldAPI mirrors the subset of the NetBox
// CustomObjectTypeField schema this resource reads and writes. JSON-typed
// attributes (default, related_object_filter) are decoded as
// json.RawMessage so we can preserve arbitrary structure and re-emit them
// verbatim on writes. Fields are ordered alphabetically by JSON tag.
type customObjectTypeFieldAPI struct {
	AppLabel                string          `json:"app_label,omitempty"`
	ChoiceSet               *int64          `json:"choice_set,omitempty"`
	Comments                string          `json:"comments,omitempty"`
	Context                 *bool           `json:"context,omitempty"`
	CustomObjectType        int64           `json:"custom_object_type"`
	Default                 json.RawMessage `json:"default,omitempty"`
	Deprecated              *bool           `json:"deprecated,omitempty"`
	DeprecatedSince         string          `json:"deprecated_since,omitempty"`
	Description             string          `json:"description,omitempty"`
	FilterLogic             string          `json:"filter_logic,omitempty"`
	GroupName               string          `json:"group_name,omitempty"`
	ID                      int64           `json:"id,omitempty"`
	IsCloneable             *bool           `json:"is_cloneable,omitempty"`
	IsPolymorphic           *bool           `json:"is_polymorphic,omitempty"`
	Label                   string          `json:"label,omitempty"`
	Model                   string          `json:"model,omitempty"`
	Name                    string          `json:"name"`
	OnDeleteBehavior        string          `json:"on_delete_behavior,omitempty"`
	Primary                 *bool           `json:"primary,omitempty"`
	RelatedName             string          `json:"related_name,omitempty"`
	RelatedObjectFilter     json.RawMessage `json:"related_object_filter,omitempty"`
	RelatedObjectType       json.RawMessage `json:"related_object_type,omitempty"`
	RelatedObjectTypes      json.RawMessage `json:"related_object_types,omitempty"`
	RelatedObjectTypesInput []appLabelModel `json:"related_object_types_input,omitempty"`
	Required                *bool           `json:"required,omitempty"`
	SchemaID                *int64          `json:"schema_id,omitempty"`
	ScheduledRemoval        string          `json:"scheduled_removal,omitempty"`
	SearchWeight            *int64          `json:"search_weight,omitempty"`
	Type                    string          `json:"type,omitempty"`
	UIEditable              string          `json:"ui_editable,omitempty"`
	UIVisible               string          `json:"ui_visible,omitempty"`
	Unique                  *bool           `json:"unique,omitempty"`
	URL                     string          `json:"url,omitempty"`
	ValidationMaximum       *int64          `json:"validation_maximum,omitempty"`
	ValidationMinimum       *int64          `json:"validation_minimum,omitempty"`
	ValidationRegex         string          `json:"validation_regex,omitempty"`
	Weight                  *int64          `json:"weight,omitempty"`
}

// appLabelModel is the {app_label, model} object used by the API's
// related_object_types_input write-only field for polymorphic references.
type appLabelModel struct {
	AppLabel string `json:"app_label"`
	Model    string `json:"model"`
}

// fieldNameRegexp enforces the NetBox pattern for
// CustomObjectTypeField.name: lowercase alphanumerics separated by
// underscores. Reused between plan validation and error messages.
var fieldNameRegexp = regexp.MustCompile(`^[a-z0-9]+(_[a-z0-9]+)*$`)

// relatedNameRegexp enforces the pattern used by
// CustomObjectTypeField.related_name.
var relatedNameRegexp = regexp.MustCompile(`^[a-z0-9_]+$`)

// dottedObjectTypeRegexp validates the "app_label.model" form used by the
// related_object_type_input and related_object_types_input attributes.
var dottedObjectTypeRegexp = regexp.MustCompile(`^[a-z0-9_]+\.[a-z0-9_]+$`)

// Enumerations reused by the schema validators.
var (
	customObjectTypeFieldTypes = []string{
		"text", "longtext", "integer", "decimal", "boolean", "date",
		"datetime", "url", "json", "select", "multiselect", "object",
		"multiobject",
	}
	customObjectTypeFieldOnDelete = []string{"set_null", "cascade", "protect", ""}
	customObjectTypeFieldFilters  = []string{"disabled", "loose", "exact"}
	customObjectTypeFieldVisible  = []string{"always", "if-set", "hidden"}
	customObjectTypeFieldEditable = []string{"yes", "no", "hidden"}
)

func (r *CustomObjectTypeFieldResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_custom_object_type_field"
}

func (r *CustomObjectTypeFieldResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a field on a NetBox Custom Object Type, provided by the " +
			"[netbox-custom-objects](https://github.com/netbox-community/netbox-custom-objects) plugin. " +
			"Fields define the individual attributes (text, integer, object reference, etc.) " +
			"available on instances of a Custom Object Type.",
		Attributes: map[string]schema.Attribute{
			"choice_set_id": schema.Int64Attribute{
				MarkdownDescription: "Numeric ID of a NetBox custom field choice set (used with " +
					"`select` and `multiselect` fields).",
				Optional: true,
				Computed: true,
			},
			"comments": schema.StringAttribute{
				MarkdownDescription: "Free-form comments on the field.",
				Optional:            true,
				Computed:            true,
			},
			"context": schema.BoolAttribute{
				MarkdownDescription: "If true, this field's value is shown as context when the object " +
					"is referenced by other objects.",
				Optional: true,
				Computed: true,
			},
			"custom_object_type_id": schema.Int64Attribute{
				MarkdownDescription: "Numeric ID of the Custom Object Type this field belongs to. " +
					"Changing this forces a new field to be created.",
				Required: true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
				},
			},
			"default": schema.StringAttribute{
				MarkdownDescription: "Default value for the field, encoded as a JSON string. " +
					"For example: `\"foo\"`, `42`, `true`, or `[\"a\",\"b\"]`. The provider " +
					"normalizes the JSON to a compact form, so equivalent formatting variants " +
					"will not produce spurious diffs.",
				Optional: true,
				Computed: true,
				Validators: []validator.String{
					jsonStringValidator{},
				},
				PlanModifiers: []planmodifier.String{
					normalizeJSONStringPlanModifier{},
				},
			},
			"deprecated": schema.BoolAttribute{
				MarkdownDescription: "If true, the field is marked deprecated: it remains in the database " +
					"but becomes read-only in the UI.",
				Optional: true,
				Computed: true,
			},
			"deprecated_since": schema.StringAttribute{
				MarkdownDescription: "Schema version in which this field was marked deprecated. Maximum 50 characters.",
				Optional:            true,
				Computed:            true,
				Validators: []validator.String{
					stringvalidator.LengthAtMost(50),
				},
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "Free-form description of the field. Maximum 200 characters.",
				Optional:            true,
				Computed:            true,
				Validators: []validator.String{
					stringvalidator.LengthAtMost(200),
				},
			},
			"filter_logic": schema.StringAttribute{
				MarkdownDescription: "Filter matching mode. One of `disabled`, `loose`, `exact`.",
				Optional:            true,
				Computed:            true,
				Validators: []validator.String{
					stringvalidator.OneOf(customObjectTypeFieldFilters...),
				},
			},
			"group_name": schema.StringAttribute{
				MarkdownDescription: "Group label used to cluster fields together in the UI. Maximum 50 characters.",
				Optional:            true,
				Computed:            true,
				Validators: []validator.String{
					stringvalidator.LengthAtMost(50),
				},
			},
			"id": schema.Int64Attribute{
				MarkdownDescription: "Numeric identifier of the Custom Object Type Field in NetBox.",
				Computed:            true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"is_cloneable": schema.BoolAttribute{
				MarkdownDescription: "If true, the field's value is replicated when cloning objects.",
				Optional:            true,
				Computed:            true,
			},
			"is_polymorphic": schema.BoolAttribute{
				MarkdownDescription: "If true, this object-typed field references multiple types " +
					"(a generic foreign key). Use `related_object_types_input` to list them.",
				Optional: true,
				Computed: true,
			},
			"label": schema.StringAttribute{
				MarkdownDescription: "Human-readable label shown in the NetBox UI. Defaults to `name` " +
					"if omitted. Maximum 50 characters.",
				Optional: true,
				Computed: true,
				Validators: []validator.String{
					stringvalidator.LengthAtMost(50),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Internal field name. Must match `^[a-z0-9]+(_[a-z0-9]+)*$` " +
					"(lowercase alphanumerics separated by underscores). Maximum 50 characters. " +
					"Changing this forces a new field to be created.",
				Required: true,
				Validators: []validator.String{
					stringvalidator.LengthBetween(1, 50),
					stringvalidator.RegexMatches(fieldNameRegexp,
						"must match ^[a-z0-9]+(_[a-z0-9]+)*$ (lowercase alphanumerics separated by underscores)"),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"on_delete_behavior": schema.StringAttribute{
				MarkdownDescription: "Behavior when the referenced object is deleted (object fields only). " +
					"One of `set_null`, `cascade`, `protect`, or an empty string.",
				Optional: true,
				Computed: true,
				Validators: []validator.String{
					stringvalidator.OneOf(customObjectTypeFieldOnDelete...),
				},
			},
			"primary": schema.BoolAttribute{
				MarkdownDescription: "If true, this field's value is used as the object's display name.",
				Optional:            true,
				Computed:            true,
			},
			"related_name": schema.StringAttribute{
				MarkdownDescription: "Reverse relation accessor name for `object`/`multiobject` fields. " +
					"Must match `^[a-z0-9_]+$`. Maximum 100 characters.",
				Optional: true,
				Computed: true,
				Validators: []validator.String{
					stringvalidator.LengthAtMost(100),
					stringvalidator.RegexMatches(relatedNameRegexp,
						"must match ^[a-z0-9_]+$"),
				},
			},
			"related_object_filter": schema.StringAttribute{
				MarkdownDescription: "Filter applied to the object selection choices, encoded as a JSON " +
					"object (`query_params` dict). For example: `{\"status\":\"active\"}`. The " +
					"provider normalizes the JSON to a compact form.",
				Optional: true,
				Computed: true,
				Validators: []validator.String{
					jsonStringValidator{},
				},
				PlanModifiers: []planmodifier.String{
					normalizeJSONStringPlanModifier{},
				},
			},
			"related_object_type": schema.StringAttribute{
				MarkdownDescription: "Dotted `app_label.model` of the referenced object type " +
					"(for non-polymorphic `object`/`multiobject` fields), as reported by NetBox.",
				Computed: true,
			},
			"related_object_type_input": schema.StringAttribute{
				MarkdownDescription: "Dotted `app_label.model` of the referenced object type for " +
					"non-polymorphic `object`/`multiobject` fields (write-only). " +
					"Split by the provider into the API's `app_label` and `model` inputs.",
				Optional: true,
				Validators: []validator.String{
					stringvalidator.RegexMatches(dottedObjectTypeRegexp,
						"must be in the form <app_label>.<model>"),
				},
			},
			"related_object_types": schema.StringAttribute{
				MarkdownDescription: "Comma-separated list of dotted `app_label.model` values for " +
					"polymorphic references, as reported by NetBox.",
				Computed: true,
			},
			"related_object_types_input": schema.SetAttribute{
				MarkdownDescription: "Set of dotted `app_label.model` values for polymorphic references " +
					"(write-only). Each entry is sent to the API as `{app_label, model}`.",
				ElementType: types.StringType,
				Optional:    true,
			},
			"required": schema.BoolAttribute{
				MarkdownDescription: "If true, this field must be set when creating or editing an object.",
				Optional:            true,
				Computed:            true,
			},
			"schema_id": schema.Int64Attribute{
				MarkdownDescription: "Stable numeric identifier for this field used during schema diffing. " +
					"Assigned by NetBox on creation.",
				Computed: true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"scheduled_removal": schema.StringAttribute{
				MarkdownDescription: "Schema version in which this field is planned to be removed. " +
					"Maximum 50 characters.",
				Optional: true,
				Computed: true,
				Validators: []validator.String{
					stringvalidator.LengthAtMost(50),
				},
			},
			"search_weight": schema.Int64Attribute{
				MarkdownDescription: "Search weight for the field. Lower values rank higher; `0` disables " +
					"the field for search. Range 0-32767.",
				Optional: true,
				Computed: true,
				Validators: []validator.Int64{
					int64validator.Between(0, 32767),
				},
			},
			"type": schema.StringAttribute{
				MarkdownDescription: "Data type of the field. One of " +
					"`text`, `longtext`, `integer`, `decimal`, `boolean`, `date`, `datetime`, " +
					"`url`, `json`, `select`, `multiselect`, `object`, `multiobject`. " +
					"Changing this forces a new field to be created.",
				Required: true,
				Validators: []validator.String{
					stringvalidator.OneOf(customObjectTypeFieldTypes...),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"ui_editable": schema.StringAttribute{
				MarkdownDescription: "Controls whether the field value can be edited in the UI. One of " +
					"`yes`, `no`, `hidden`.",
				Optional: true,
				Computed: true,
				Validators: []validator.String{
					stringvalidator.OneOf(customObjectTypeFieldEditable...),
				},
			},
			"ui_visible": schema.StringAttribute{
				MarkdownDescription: "Controls whether the field is displayed in the UI. One of " +
					"`always`, `if-set`, `hidden`.",
				Optional: true,
				Computed: true,
				Validators: []validator.String{
					stringvalidator.OneOf(customObjectTypeFieldVisible...),
				},
			},
			"unique": schema.BoolAttribute{
				MarkdownDescription: "If true, values of this field must be unique across all instances " +
					"of the parent Custom Object Type.",
				Optional: true,
				Computed: true,
			},
			"url": schema.StringAttribute{
				MarkdownDescription: "API URL of this Custom Object Type Field as reported by NetBox.",
				Computed:            true,
			},
			"validation_maximum": schema.Int64Attribute{
				MarkdownDescription: "Maximum allowed value for numeric fields.",
				Optional:            true,
				Computed:            true,
			},
			"validation_minimum": schema.Int64Attribute{
				MarkdownDescription: "Minimum allowed value for numeric fields.",
				Optional:            true,
				Computed:            true,
			},
			"validation_regex": schema.StringAttribute{
				MarkdownDescription: "Regular expression enforced on text field values. Maximum 500 characters.",
				Optional:            true,
				Computed:            true,
				Validators: []validator.String{
					stringvalidator.LengthAtMost(500),
				},
			},
			"weight": schema.Int64Attribute{
				MarkdownDescription: "Display weight; higher weights appear lower in forms. Range 0-32767.",
				Optional:            true,
				Computed:            true,
				Validators: []validator.Int64{
					int64validator.Between(0, 32767),
				},
			},
		},
	}
}

func (r *CustomObjectTypeFieldResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	client, ok := req.ProviderData.(*Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *provider.Client, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}
	r.client = client
}

func (r *CustomObjectTypeFieldResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan CustomObjectTypeFieldResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body, diags := fieldPlanToAPI(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	var created customObjectTypeFieldAPI
	if err := r.client.DoJSON(ctx, http.MethodPost, customObjectTypeFieldsPath, body, &created); err != nil {
		resp.Diagnostics.AddError(
			"Unable to create NetBox custom object type field",
			err.Error(),
		)
		return
	}

	tflog.Trace(ctx, "created netbox custom object type field", map[string]any{"id": created.ID})

	diags = fieldAPIToState(ctx, &created, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *CustomObjectTypeFieldResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state CustomObjectTypeFieldResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if state.ID.IsNull() || state.ID.IsUnknown() {
		resp.State.RemoveResource(ctx)
		return
	}

	var fetched customObjectTypeFieldAPI
	err := r.client.DoJSON(ctx, http.MethodGet, fieldResourcePath(state.ID.ValueInt64()), nil, &fetched)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			tflog.Debug(ctx, "netbox custom object type field no longer exists; removing from state",
				map[string]any{"id": state.ID.ValueInt64()})
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Unable to read NetBox custom object type field",
			err.Error(),
		)
		return
	}

	diags := fieldAPIToState(ctx, &fetched, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *CustomObjectTypeFieldResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan CustomObjectTypeFieldResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state CustomObjectTypeFieldResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Preserve the existing ID; it is not part of the request body but
	// determines the URL path we PUT to.
	plan.ID = state.ID

	body, diags := fieldPlanToAPI(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	var updated customObjectTypeFieldAPI
	if err := r.client.DoJSON(ctx, http.MethodPut, fieldResourcePath(state.ID.ValueInt64()), body, &updated); err != nil {
		resp.Diagnostics.AddError(
			"Unable to update NetBox custom object type field",
			err.Error(),
		)
		return
	}

	diags = fieldAPIToState(ctx, &updated, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *CustomObjectTypeFieldResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state CustomObjectTypeFieldResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if state.ID.IsNull() || state.ID.IsUnknown() {
		return
	}

	err := r.client.DoJSON(ctx, http.MethodDelete, fieldResourcePath(state.ID.ValueInt64()), nil, nil)
	if err != nil && !errors.Is(err, ErrNotFound) {
		resp.Diagnostics.AddError(
			"Unable to delete NetBox custom object type field",
			err.Error(),
		)
		return
	}
}

func (r *CustomObjectTypeFieldResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	id, err := strconv.ParseInt(req.ID, 10, 64)
	if err != nil {
		resp.Diagnostics.AddError(
			"Invalid import ID",
			fmt.Sprintf("Expected the numeric NetBox Custom Object Type Field ID, got %q: %s", req.ID, err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), id)...)
}

// fieldResourcePath returns the item URL for a given Custom Object Type
// Field ID.
func fieldResourcePath(id int64) string {
	return fmt.Sprintf("%s%d/", customObjectTypeFieldsPath, id)
}

// fieldPlanToAPI translates the Terraform plan into the request body
// accepted by POST/PUT on the Custom Object Type Fields endpoint. Only
// values the user explicitly set (non-null, non-unknown) are forwarded
// so NetBox can apply its own defaults on Create and preserve unchanged
// attributes on Update.
func fieldPlanToAPI(ctx context.Context, plan *CustomObjectTypeFieldResourceModel) (*customObjectTypeFieldAPI, diag.Diagnostics) {
	var diags diag.Diagnostics
	out := &customObjectTypeFieldAPI{
		CustomObjectType: plan.CustomObjectTypeID.ValueInt64(),
		Name:             plan.Name.ValueString(),
		Type:             plan.Type.ValueString(),
	}

	setString(&out.Comments, plan.Comments)
	setString(&out.DeprecatedSince, plan.DeprecatedSince)
	setString(&out.Description, plan.Description)
	setString(&out.FilterLogic, plan.FilterLogic)
	setString(&out.GroupName, plan.GroupName)
	setString(&out.Label, plan.Label)
	setString(&out.OnDeleteBehavior, plan.OnDeleteBehavior)
	setString(&out.RelatedName, plan.RelatedName)
	setString(&out.ScheduledRemoval, plan.ScheduledRemoval)
	setString(&out.UIEditable, plan.UIEditable)
	setString(&out.UIVisible, plan.UIVisible)
	setString(&out.ValidationRegex, plan.ValidationRegex)

	out.Context = boolPtr(plan.Context)
	out.Deprecated = boolPtr(plan.Deprecated)
	out.IsCloneable = boolPtr(plan.IsCloneable)
	out.IsPolymorphic = boolPtr(plan.IsPolymorphic)
	out.Primary = boolPtr(plan.Primary)
	out.Required = boolPtr(plan.Required)
	out.Unique = boolPtr(plan.Unique)

	out.ChoiceSet = int64Ptr(plan.ChoiceSetID)
	out.SearchWeight = int64Ptr(plan.SearchWeight)
	out.ValidationMaximum = int64Ptr(plan.ValidationMaximum)
	out.ValidationMinimum = int64Ptr(plan.ValidationMinimum)
	out.Weight = int64Ptr(plan.Weight)

	// JSON-typed attributes: forward the raw JSON bytes as-is so nested
	// structure is preserved.
	if raw, ok := jsonRawFromString(plan.Default); ok {
		out.Default = raw
	}
	if raw, ok := jsonRawFromString(plan.RelatedObjectFilter); ok {
		out.RelatedObjectFilter = raw
	}

	// Object / MultiObject reference inputs. The API accepts either a
	// single (app_label, model) pair or a polymorphic list. We translate
	// the friendlier dotted form used by the resource into the wire
	// format.
	if !plan.RelatedObjectTypeInput.IsNull() && !plan.RelatedObjectTypeInput.IsUnknown() {
		app, model, err := splitDottedObjectType(plan.RelatedObjectTypeInput.ValueString())
		if err != nil {
			diags.AddAttributeError(
				path.Root("related_object_type_input"),
				"Invalid related_object_type_input",
				err.Error(),
			)
		} else {
			out.AppLabel = app
			out.Model = model
		}
	}

	if !plan.RelatedObjectTypesInput.IsNull() && !plan.RelatedObjectTypesInput.IsUnknown() {
		var dotted []string
		d := plan.RelatedObjectTypesInput.ElementsAs(ctx, &dotted, false)
		diags.Append(d...)
		if !diags.HasError() {
			out.RelatedObjectTypesInput = make([]appLabelModel, 0, len(dotted))
			for _, entry := range dotted {
				app, model, err := splitDottedObjectType(entry)
				if err != nil {
					diags.AddAttributeError(
						path.Root("related_object_types_input"),
						"Invalid related_object_types_input entry",
						err.Error(),
					)
					continue
				}
				out.RelatedObjectTypesInput = append(out.RelatedObjectTypesInput, appLabelModel{
					AppLabel: app,
					Model:    model,
				})
			}
		}
	}

	return out, diags
}

// fieldAPIToState copies the decoded API response back into the
// Terraform state model. Write-only helper attributes
// (related_object_type_input, related_object_types_input) are left
// untouched so the Terraform config value is preserved verbatim.
func fieldAPIToState(ctx context.Context, api *customObjectTypeFieldAPI, state *CustomObjectTypeFieldResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics

	state.ChoiceSetID = int64FromPtr(api.ChoiceSet)
	state.Comments = types.StringValue(api.Comments)
	state.Context = boolFromPtr(api.Context)
	state.CustomObjectTypeID = types.Int64Value(api.CustomObjectType)
	state.Default = stringFromRawJSON(api.Default)
	state.Deprecated = boolFromPtr(api.Deprecated)
	state.DeprecatedSince = types.StringValue(api.DeprecatedSince)
	state.Description = types.StringValue(api.Description)
	state.FilterLogic = types.StringValue(api.FilterLogic)
	state.GroupName = types.StringValue(api.GroupName)
	state.ID = types.Int64Value(api.ID)
	state.IsCloneable = boolFromPtr(api.IsCloneable)
	state.IsPolymorphic = boolFromPtr(api.IsPolymorphic)
	state.Label = types.StringValue(api.Label)
	state.Name = types.StringValue(api.Name)
	state.OnDeleteBehavior = types.StringValue(api.OnDeleteBehavior)
	state.Primary = boolFromPtr(api.Primary)
	state.RelatedName = types.StringValue(api.RelatedName)
	state.RelatedObjectFilter = stringFromRawJSON(api.RelatedObjectFilter)
	state.RelatedObjectType = stringifyRawJSON(api.RelatedObjectType)
	state.RelatedObjectTypes = stringifyRawJSON(api.RelatedObjectTypes)
	state.Required = boolFromPtr(api.Required)
	state.SchemaID = int64FromPtr(api.SchemaID)
	state.ScheduledRemoval = types.StringValue(api.ScheduledRemoval)
	state.SearchWeight = int64FromPtr(api.SearchWeight)
	state.Type = types.StringValue(api.Type)
	state.UIEditable = types.StringValue(api.UIEditable)
	state.UIVisible = types.StringValue(api.UIVisible)
	state.Unique = boolFromPtr(api.Unique)
	state.URL = types.StringValue(api.URL)
	state.ValidationMaximum = int64FromPtr(api.ValidationMaximum)
	state.ValidationMinimum = int64FromPtr(api.ValidationMinimum)
	state.ValidationRegex = types.StringValue(api.ValidationRegex)
	state.Weight = int64FromPtr(api.Weight)

	// Ensure write-only helper attributes always hold a concrete value in
	// state (either what the user set or a typed null) to avoid framework
	// "unknown after apply" errors.
	if state.RelatedObjectTypeInput.IsUnknown() {
		state.RelatedObjectTypeInput = types.StringNull()
	}
	if state.RelatedObjectTypesInput.IsUnknown() {
		state.RelatedObjectTypesInput = types.SetNull(types.StringType)
	}

	_ = ctx
	return diags
}

// setString forwards a Terraform string attribute to a Go string field
// only if the user actually provided a value (non-null, non-unknown).
func setString(dst *string, v types.String) {
	if v.IsNull() || v.IsUnknown() {
		return
	}
	*dst = v.ValueString()
}

// boolPtr converts a Terraform bool attribute into a *bool suitable for
// omitempty JSON encoding. Null/unknown maps to nil so the field is
// omitted from the request body.
func boolPtr(v types.Bool) *bool {
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	b := v.ValueBool()
	return &b
}

// int64Ptr converts a Terraform int64 attribute into a *int64. Null or
// unknown values map to nil so the field is omitted from the request.
func int64Ptr(v types.Int64) *int64 {
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	i := v.ValueInt64()
	return &i
}

// boolFromPtr converts a *bool from the API into a Terraform bool value,
// treating nil as null.
func boolFromPtr(p *bool) types.Bool {
	if p == nil {
		return types.BoolNull()
	}
	return types.BoolValue(*p)
}

// int64FromPtr converts a *int64 from the API into a Terraform int64
// value, treating nil as null.
func int64FromPtr(p *int64) types.Int64 {
	if p == nil {
		return types.Int64Null()
	}
	return types.Int64Value(*p)
}

// jsonRawFromString returns the raw JSON bytes of a user-supplied string
// attribute, ready to be forwarded to the API. The second return value
// is false if the attribute is null/unknown and should be omitted.
func jsonRawFromString(v types.String) (json.RawMessage, bool) {
	if v.IsNull() || v.IsUnknown() {
		return nil, false
	}
	s := strings.TrimSpace(v.ValueString())
	if s == "" {
		return nil, false
	}
	return json.RawMessage(s), true
}

// stringFromRawJSON returns a Terraform string value holding the compact
// JSON encoding of the API-provided raw message, or null when the
// message is empty. The compact form gives a stable comparison target on
// subsequent refreshes.
func stringFromRawJSON(raw json.RawMessage) types.String {
	if len(raw) == 0 {
		return types.StringNull()
	}
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return types.StringNull()
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		// Preserve whatever the server sent so users can at least see the
		// raw value rather than losing it silently.
		return types.StringValue(trimmed)
	}
	compact, err := json.Marshal(v)
	if err != nil {
		return types.StringValue(trimmed)
	}
	return types.StringValue(string(compact))
}

// stringifyRawJSON turns an arbitrary JSON value returned by the API into
// a Terraform string suitable for a read-only informational attribute.
// A JSON string is returned unquoted so users see e.g. `dcim.device`
// rather than `"dcim.device"`; arrays and objects are compacted; null
// and empty messages map to a null Terraform value.
func stringifyRawJSON(raw json.RawMessage) types.String {
	if len(raw) == 0 {
		return types.StringNull()
	}
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return types.StringNull()
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return types.StringValue(s)
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return types.StringValue(trimmed)
	}
	compact, err := json.Marshal(v)
	if err != nil {
		return types.StringValue(trimmed)
	}
	return types.StringValue(string(compact))
}

// splitDottedObjectType parses a "app_label.model" string into its two
// components. It returns a descriptive error when the input does not
// match the expected pattern.
func splitDottedObjectType(dotted string) (string, string, error) {
	if !dottedObjectTypeRegexp.MatchString(dotted) {
		return "", "", fmt.Errorf("expected <app_label>.<model> (e.g. dcim.device), got %q", dotted)
	}
	parts := strings.SplitN(dotted, ".", 2)
	return parts[0], parts[1], nil
}

// jsonStringValidator ensures that an attribute holding JSON is
// syntactically valid so misconfigurations are caught at plan time
// rather than after a failed API request.
type jsonStringValidator struct{}

func (jsonStringValidator) Description(_ context.Context) string {
	return "value must be a valid JSON document"
}

func (jsonStringValidator) MarkdownDescription(_ context.Context) string {
	return "value must be a valid JSON document"
}

func (jsonStringValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	s := strings.TrimSpace(req.ConfigValue.ValueString())
	if s == "" {
		return
	}
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Invalid JSON",
			fmt.Sprintf("Value must be a valid JSON document: %s", err),
		)
	}
}

// normalizeJSONStringPlanModifier rewrites a JSON-valued string attribute
// to its compact canonical form during planning. Users can supply JSON
// with arbitrary whitespace in their configuration without producing
// spurious diffs against the compact form the API returns.
type normalizeJSONStringPlanModifier struct{}

func (normalizeJSONStringPlanModifier) Description(_ context.Context) string {
	return "Normalizes JSON-valued strings to their compact canonical form."
}

func (normalizeJSONStringPlanModifier) MarkdownDescription(_ context.Context) string {
	return "Normalizes JSON-valued strings to their compact canonical form."
}

func (normalizeJSONStringPlanModifier) PlanModifyString(_ context.Context, req planmodifier.StringRequest, resp *planmodifier.StringResponse) {
	if req.PlanValue.IsNull() || req.PlanValue.IsUnknown() {
		return
	}
	s := strings.TrimSpace(req.PlanValue.ValueString())
	if s == "" {
		return
	}
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		// Let the validator surface the parse error; leave the plan
		// value unchanged.
		return
	}
	compact, err := json.Marshal(v)
	if err != nil {
		return
	}
	resp.PlanValue = types.StringValue(string(compact))
}
