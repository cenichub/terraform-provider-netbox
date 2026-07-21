// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// Ensure provider defined types fully satisfy framework interfaces.
var (
	_ resource.Resource                = &CustomObjectTypeResource{}
	_ resource.ResourceWithImportState = &CustomObjectTypeResource{}
	_ resource.ResourceWithConfigure   = &CustomObjectTypeResource{}
)

const customObjectTypesPath = "/api/plugins/custom-objects/custom-object-types/"

// NewCustomObjectTypeResource returns a factory for the Custom Object Type
// resource. It is registered on the provider from Resources().
func NewCustomObjectTypeResource() resource.Resource {
	return &CustomObjectTypeResource{}
}

// CustomObjectTypeResource manages a NetBox Custom Object Type provided
// by the netbox-custom-objects plugin. The upstream endpoint is
// /api/plugins/custom-objects/custom-object-types/.
type CustomObjectTypeResource struct {
	client *Client
}

// CustomObjectTypeResourceModel describes the Terraform state for a
// Custom Object Type. All fields map 1:1 to the NetBox
// CustomObjectType/CustomObjectTypeRequest schemas.
type CustomObjectTypeResourceModel struct {
	Created           types.String `tfsdk:"created"`
	Description       types.String `tfsdk:"description"`
	GroupName         types.String `tfsdk:"group_name"`
	ID                types.Int64  `tfsdk:"id"`
	LastUpdated       types.String `tfsdk:"last_updated"`
	Name              types.String `tfsdk:"name"`
	ObjectTypeName    types.String `tfsdk:"object_type_name"`
	Slug              types.String `tfsdk:"slug"`
	TableModelName    types.String `tfsdk:"table_model_name"`
	Tags              types.Set    `tfsdk:"tags"`
	URL               types.String `tfsdk:"url"`
	VerboseName       types.String `tfsdk:"verbose_name"`
	VerboseNamePlural types.String `tfsdk:"verbose_name_plural"`
	Version           types.String `tfsdk:"version"`
}

// customObjectTypeAPI mirrors the subset of the NetBox CustomObjectType
// schema that this resource reads and writes. Read-only fields are
// decoded from the API response and surfaced as Computed attributes.
type customObjectTypeAPI struct {
	Created           string      `json:"created,omitempty"`
	Description       string      `json:"description,omitempty"`
	GroupName         string      `json:"group_name,omitempty"`
	ID                int64       `json:"id,omitempty"`
	LastUpdated       string      `json:"last_updated,omitempty"`
	Name              string      `json:"name"`
	ObjectTypeName    string      `json:"object_type_name,omitempty"`
	Slug              string      `json:"slug"`
	TableModelName    string      `json:"table_model_name,omitempty"`
	Tags              []nestedTag `json:"tags,omitempty"`
	URL               string      `json:"url,omitempty"`
	VerboseName       string      `json:"verbose_name,omitempty"`
	VerboseNamePlural string      `json:"verbose_name_plural,omitempty"`
	Version           string      `json:"version,omitempty"`
}

// nestedTag represents both the read shape (NestedTag with id/name/slug)
// and the minimal write shape (NestedTagRequest with name/slug). On
// write NetBox accepts either a PK or a { name, slug } dictionary; we
// always send { slug } since slugs are unique and user-facing.
type nestedTag struct {
	ID   int64  `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
	Slug string `json:"slug,omitempty"`
}

// nameRegexp enforces the NetBox pattern for CustomObjectType.name:
// lowercase alphanumerics separated by underscores.
var nameRegexp = regexp.MustCompile(`^[a-z0-9]+(_[a-z0-9]+)*$`)

// slugRegexp enforces the NetBox pattern for CustomObjectType.slug.
var slugRegexp = regexp.MustCompile(`^[-a-zA-Z0-9_]+$`)

// tagSlugRegexp enforces the pattern used by NetBox tag slugs, which
// mirrors Django's default slug validator (\w and hyphens).
var tagSlugRegexp = regexp.MustCompile(`^[-\w]+$`)

func (r *CustomObjectTypeResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_custom_object_type"
}

func (r *CustomObjectTypeResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a NetBox Custom Object Type, provided by the " +
			"[netbox-custom-objects](https://github.com/netbox-community/netbox-custom-objects) plugin. " +
			"A Custom Object Type defines a new object model in NetBox (its name, slug, and metadata); " +
			"individual fields on the type are managed with a separate resource.",
		Attributes: map[string]schema.Attribute{
			"created": schema.StringAttribute{
				MarkdownDescription: "RFC3339 timestamp when the type was created.",
				Computed:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "Free-form description of the type. Maximum 200 characters.",
				Optional:            true,
				Computed:            true,
				Validators: []validator.String{
					stringvalidator.LengthAtMost(200),
				},
			},
			"group_name": schema.StringAttribute{
				MarkdownDescription: "Group label used to cluster similar Custom Object Types in the " +
					"NetBox navigation menu.",
				Optional: true,
				Computed: true,
				Validators: []validator.String{
					stringvalidator.LengthAtMost(100),
				},
			},
			"id": schema.Int64Attribute{
				MarkdownDescription: "Numeric identifier of the Custom Object Type in NetBox.",
				Computed:            true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"last_updated": schema.StringAttribute{
				MarkdownDescription: "RFC3339 timestamp of the last modification to the type.",
				Computed:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Internal name of the type. Must match `^[a-z0-9]+(_[a-z0-9]+)*$` " +
					"(lowercase alphanumerics separated by underscores). Maximum 100 characters.",
				Required: true,
				Validators: []validator.String{
					stringvalidator.LengthBetween(1, 100),
					stringvalidator.RegexMatches(nameRegexp,
						"must match ^[a-z0-9]+(_[a-z0-9]+)*$ (lowercase alphanumerics separated by underscores)"),
				},
			},
			"object_type_name": schema.StringAttribute{
				MarkdownDescription: "Dotted `app_label.model` name that NetBox uses to reference " +
					"instances of this type internally.",
				Computed: true,
			},
			"slug": schema.StringAttribute{
				MarkdownDescription: "URL-friendly slug for the type. Must match `^[-a-zA-Z0-9_]+$`. " +
					"Maximum 100 characters.",
				Required: true,
				Validators: []validator.String{
					stringvalidator.LengthBetween(1, 100),
					stringvalidator.RegexMatches(slugRegexp,
						"must match ^[-a-zA-Z0-9_]+$"),
				},
			},
			"table_model_name": schema.StringAttribute{
				MarkdownDescription: "Dynamically generated Django model class name backing the type.",
				Computed:            true,
			},
			"tags": schema.SetAttribute{
				MarkdownDescription: "Set of NetBox tag slugs to associate with the type. " +
					"Each entry must be an existing tag slug.",
				ElementType: types.StringType,
				Optional:    true,
				Validators:  []validator.Set{
					// Per-element validation is left to NetBox itself; we
					// only enforce that the strings look like plausible
					// tag slugs to catch obvious typos client-side.
				},
			},
			"url": schema.StringAttribute{
				MarkdownDescription: "API URL of this Custom Object Type as reported by NetBox.",
				Computed:            true,
			},
			"verbose_name": schema.StringAttribute{
				MarkdownDescription: "Human-readable singular name shown in the NetBox UI.",
				Optional:            true,
				Computed:            true,
				Validators: []validator.String{
					stringvalidator.LengthAtMost(100),
				},
			},
			"verbose_name_plural": schema.StringAttribute{
				MarkdownDescription: "Human-readable plural name shown in the NetBox UI.",
				Optional:            true,
				Computed:            true,
				Validators: []validator.String{
					stringvalidator.LengthAtMost(100),
				},
			},
			"version": schema.StringAttribute{
				MarkdownDescription: "Free-form version string associated with the type's schema.",
				Optional:            true,
				Computed:            true,
				Validators: []validator.String{
					stringvalidator.LengthAtMost(50),
				},
			},
		},
	}
}

func (r *CustomObjectTypeResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *CustomObjectTypeResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan CustomObjectTypeResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body, diags := planToAPI(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	var created customObjectTypeAPI
	if err := r.client.DoJSON(ctx, http.MethodPost, customObjectTypesPath, body, &created); err != nil {
		resp.Diagnostics.AddError(
			"Unable to create NetBox custom object type",
			err.Error(),
		)
		return
	}

	tflog.Trace(ctx, "created netbox custom object type", map[string]any{"id": created.ID})

	diags = apiToState(ctx, &created, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *CustomObjectTypeResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state CustomObjectTypeResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if state.ID.IsNull() || state.ID.IsUnknown() {
		resp.State.RemoveResource(ctx)
		return
	}

	var fetched customObjectTypeAPI
	err := r.client.DoJSON(ctx, http.MethodGet, resourcePath(state.ID.ValueInt64()), nil, &fetched)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			tflog.Debug(ctx, "netbox custom object type no longer exists; removing from state",
				map[string]any{"id": state.ID.ValueInt64()})
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Unable to read NetBox custom object type",
			err.Error(),
		)
		return
	}

	diags := apiToState(ctx, &fetched, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *CustomObjectTypeResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan CustomObjectTypeResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state CustomObjectTypeResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Preserve the existing ID; it is not part of the request body but
	// determines the URL path we PUT to.
	plan.ID = state.ID

	body, diags := planToAPI(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	var updated customObjectTypeAPI
	if err := r.client.DoJSON(ctx, http.MethodPut, resourcePath(state.ID.ValueInt64()), body, &updated); err != nil {
		resp.Diagnostics.AddError(
			"Unable to update NetBox custom object type",
			err.Error(),
		)
		return
	}

	diags = apiToState(ctx, &updated, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *CustomObjectTypeResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state CustomObjectTypeResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if state.ID.IsNull() || state.ID.IsUnknown() {
		return
	}

	err := r.client.DoJSON(ctx, http.MethodDelete, resourcePath(state.ID.ValueInt64()), nil, nil)
	if err != nil && !errors.Is(err, ErrNotFound) {
		resp.Diagnostics.AddError(
			"Unable to delete NetBox custom object type",
			err.Error(),
		)
		return
	}
}

func (r *CustomObjectTypeResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	id, err := strconv.ParseInt(req.ID, 10, 64)
	if err != nil {
		resp.Diagnostics.AddError(
			"Invalid import ID",
			fmt.Sprintf("Expected the numeric NetBox Custom Object Type ID, got %q: %s", req.ID, err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), id)...)
}

// resourcePath returns the item URL for a given Custom Object Type ID.
func resourcePath(id int64) string {
	return fmt.Sprintf("%s%d/", customObjectTypesPath, id)
}

// planToAPI translates the Terraform plan into the request body accepted
// by POST/PUT on the Custom Object Types endpoint.
func planToAPI(ctx context.Context, plan *CustomObjectTypeResourceModel) (*customObjectTypeAPI, diag.Diagnostics) {
	var diags diag.Diagnostics
	out := &customObjectTypeAPI{
		Description:       plan.Description.ValueString(),
		GroupName:         plan.GroupName.ValueString(),
		Name:              plan.Name.ValueString(),
		Slug:              plan.Slug.ValueString(),
		VerboseName:       plan.VerboseName.ValueString(),
		VerboseNamePlural: plan.VerboseNamePlural.ValueString(),
		Version:           plan.Version.ValueString(),
	}

	if !plan.Tags.IsNull() && !plan.Tags.IsUnknown() {
		var slugs []string
		d := plan.Tags.ElementsAs(ctx, &slugs, false)
		diags.Append(d...)
		if diags.HasError() {
			return nil, diags
		}
		// Force an empty (non-nil) slice when the user explicitly sets
		// tags = []: NetBox interprets omission vs. empty differently on
		// PUT (omission preserves existing tags, [] clears them).
		out.Tags = make([]nestedTag, 0, len(slugs))
		for _, s := range slugs {
			if !tagSlugRegexp.MatchString(s) {
				diags.AddAttributeError(
					path.Root("tags"),
					"Invalid tag slug",
					fmt.Sprintf("Tag slug %q does not match the expected pattern ^[-\\w]+$", s),
				)
				continue
			}
			out.Tags = append(out.Tags, nestedTag{Slug: s})
		}
	}

	return out, diags
}

// apiToState copies the decoded API response back into the Terraform
// state model, converting empty strings/nils to Terraform null values
// only where the API itself models the field as nullable/optional.
func apiToState(ctx context.Context, api *customObjectTypeAPI, state *CustomObjectTypeResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics

	state.Created = types.StringValue(api.Created)
	state.Description = types.StringValue(api.Description)
	state.GroupName = types.StringValue(api.GroupName)
	state.ID = types.Int64Value(api.ID)
	state.LastUpdated = types.StringValue(api.LastUpdated)
	state.Name = types.StringValue(api.Name)
	state.ObjectTypeName = types.StringValue(api.ObjectTypeName)
	state.Slug = types.StringValue(api.Slug)
	state.TableModelName = types.StringValue(api.TableModelName)
	state.URL = types.StringValue(api.URL)
	state.VerboseName = types.StringValue(api.VerboseName)
	state.VerboseNamePlural = types.StringValue(api.VerboseNamePlural)
	state.Version = types.StringValue(api.Version)

	// When the API returns no tags, surface state as null (rather than an
	// empty set) so plans that omit `tags` remain consistent with state
	// after apply.
	if len(api.Tags) == 0 {
		state.Tags = types.SetNull(types.StringType)
		return diags
	}

	slugs := make([]string, 0, len(api.Tags))
	for _, t := range api.Tags {
		slugs = append(slugs, t.Slug)
	}
	tagSet, d := types.SetValueFrom(ctx, types.StringType, slugs)
	diags.Append(d...)
	if diags.HasError() {
		return diags
	}
	state.Tags = tagSet
	return diags
}
