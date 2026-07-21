// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"os"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// Ensure NetboxProvider satisfies the provider interface.
var (
	_ provider.Provider = &NetboxProvider{}
)

// Environment variable names honored by the provider when the matching
// configuration attribute is not set.
const (
	envNetboxURL      = "NETBOX_URL"
	envNetboxToken    = "NETBOX_TOKEN" //nolint:gosec // env var name, not a secret
	envNetboxInsecure = "NETBOX_INSECURE"
)

// NetboxProvider defines the provider implementation.
type NetboxProvider struct {
	// version is set to the provider version on release, "dev" when the
	// provider is built and ran locally, and "test" when running acceptance
	// testing.
	version string
}

// NetboxProviderModel describes the provider data model.
type NetboxProviderModel struct {
	URL      types.String `tfsdk:"url"`
	Token    types.String `tfsdk:"token"`
	Insecure types.Bool   `tfsdk:"insecure"`
}

func (p *NetboxProvider) Metadata(ctx context.Context, req provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "netbox"
	resp.Version = p.version
}

func (p *NetboxProvider) Schema(ctx context.Context, req provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "The NetBox provider manages resources in a [NetBox](https://netbox.dev) instance " +
			"via its REST API. Configure it with the base URL of your NetBox server and an API token.",
		Attributes: map[string]schema.Attribute{
			"url": schema.StringAttribute{
				MarkdownDescription: "Base URL of the NetBox server, for example `https://netbox.example.com`. " +
					"May also be provided via the `" + envNetboxURL + "` environment variable.",
				Optional: true,
			},
			"token": schema.StringAttribute{
				MarkdownDescription: "NetBox API token used for authentication. " +
					"May also be provided via the `" + envNetboxToken + "` environment variable.",
				Optional:  true,
				Sensitive: true,
			},
			"insecure": schema.BoolAttribute{
				MarkdownDescription: "If `true`, skip TLS certificate verification when talking to NetBox. " +
					"Defaults to `false`. May also be provided via the `" + envNetboxInsecure + "` " +
					"environment variable (`1`, `true`, `yes` enable it).",
				Optional: true,
			},
		},
	}
}

func (p *NetboxProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var data NetboxProviderModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Reject unknown values with attribute-scoped diagnostics so the CLI
	// can point the user at the offending configuration.
	if data.URL.IsUnknown() {
		resp.Diagnostics.AddAttributeError(
			path.Root("url"),
			"Unknown NetBox URL",
			"The provider cannot create a NetBox API client because the URL value is unknown. "+
				"Either target apply the source of the value first, or set the value statically.",
		)
	}
	if data.Token.IsUnknown() {
		resp.Diagnostics.AddAttributeError(
			path.Root("token"),
			"Unknown NetBox API token",
			"The provider cannot create a NetBox API client because the token value is unknown. "+
				"Either target apply the source of the value first, or set the value statically or via the "+
				envNetboxToken+" environment variable.",
		)
	}
	if resp.Diagnostics.HasError() {
		return
	}

	// Resolve values: explicit config wins over environment variables.
	nbURL := os.Getenv(envNetboxURL)
	if !data.URL.IsNull() {
		nbURL = data.URL.ValueString()
	}

	nbToken := os.Getenv(envNetboxToken)
	if !data.Token.IsNull() {
		nbToken = data.Token.ValueString()
	}

	insecure := parseBoolEnv(os.Getenv(envNetboxInsecure))
	if !data.Insecure.IsNull() {
		insecure = data.Insecure.ValueBool()
	}

	if nbURL == "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("url"),
			"Missing NetBox URL",
			"The provider requires a NetBox URL. Set the `url` attribute in the provider block "+
				"or the "+envNetboxURL+" environment variable.",
		)
	}
	if nbToken == "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("token"),
			"Missing NetBox API token",
			"The provider requires a NetBox API token. Set the `token` attribute in the provider block "+
				"or the "+envNetboxToken+" environment variable.",
		)
	}
	if resp.Diagnostics.HasError() {
		return
	}

	ctx = tflog.SetField(ctx, "netbox_url", nbURL)
	ctx = tflog.MaskFieldValuesWithFieldKeys(ctx, "netbox_token")
	tflog.Debug(ctx, "creating netbox API client")

	client, err := NewClient(nbURL, nbToken, insecure)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to create NetBox client",
			"An error occurred while constructing the NetBox API client: "+err.Error(),
		)
		return
	}

	// Verify connectivity + credentials up front so misconfiguration is
	// surfaced during `terraform plan` rather than at first resource use.
	if err := client.Ping(ctx); err != nil {
		resp.Diagnostics.AddError(
			"Unable to connect to NetBox",
			"The provider was unable to verify connectivity to NetBox: "+err.Error(),
		)
		return
	}

	tflog.Info(ctx, "configured netbox API client")

	resp.DataSourceData = client
	resp.ResourceData = client
}

func (p *NetboxProvider) Resources(ctx context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewCustomObjectTypeResource,
		NewCustomObjectTypeFieldResource,
	}
}

func (p *NetboxProvider) DataSources(ctx context.Context) []func() datasource.DataSource {
	return nil
}

// New returns a factory for the NetBox provider.
func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &NetboxProvider{
			version: version,
		}
	}
}

// parseBoolEnv parses common truthy representations used in environment
// variables. Empty or unrecognized values return false.
func parseBoolEnv(v string) bool {
	if v == "" {
		return false
	}
	if b, err := strconv.ParseBool(v); err == nil {
		return b
	}
	switch v {
	case "yes", "YES", "Yes", "on", "ON", "On":
		return true
	}
	return false
}
