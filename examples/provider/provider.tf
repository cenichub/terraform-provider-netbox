terraform {
  required_providers {
    netbox = {
      source = "cenichub/netbox"
    }
  }
}

provider "netbox" {
  # The base URL of your NetBox server. Can also be set via NETBOX_URL.
  url = "https://netbox.example.com"

  # The NetBox API token. Prefer supplying this via the NETBOX_TOKEN
  # environment variable so it is not committed to version control.
  # token = "0123456789abcdef0123456789abcdef01234567"

  # Set to true to skip TLS certificate verification (development only).
  # insecure = false
}
