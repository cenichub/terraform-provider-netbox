resource "netbox_custom_object_type" "example" {
  name = "server_role"
  slug = "server-role"
}

resource "netbox_custom_object_type_field" "label" {
  custom_object_type_id = netbox_custom_object_type.example.id
  description           = "Short human-readable label for the role."
  filter_logic          = "loose"
  label                 = "Label"
  name                  = "label"
  primary               = true
  required              = true
  type                  = "text"
  ui_editable           = "yes"
  ui_visible            = "always"
  unique                = true
}

resource "netbox_custom_object_type_field" "owner" {
  custom_object_type_id     = netbox_custom_object_type.example.id
  label                     = "Owner"
  name                      = "owner"
  on_delete_behavior        = "set_null"
  related_name              = "server_role_owner"
  related_object_type_input = "tenancy.tenant"
  type                      = "object"
}
