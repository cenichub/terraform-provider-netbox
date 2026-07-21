resource "netbox_custom_object_type" "example" {
  description         = "Logical role assigned to a server."
  group_name          = "Inventory"
  name                = "server_role"
  slug                = "server-role"
  tags                = ["managed-by-terraform"]
  verbose_name        = "Server Role"
  verbose_name_plural = "Server Roles"
  version             = "1.0.0"
}
