# terraform-provider-hcloudplus

Terraform provider for Hetzner Cloud's **undocumented console API** — the
account-level `_`-prefixed routes that [`hetznercloud/terraform-provider-hcloud`](https://github.com/hetznercloud/terraform-provider-hcloud)
can't reach.

Built on [hcloud-go-plus](https://github.com/hatemosphere/hcloud-go-plus).

## What this lets you do that upstream can't

- Create, rename and delete **projects**
- Create and delete **project API tokens** (and capture the secret for use
  with the upstream `hcloud` provider)
- Invite, resend and revoke **project members**
- Read **account-wide resource limits** and **current user** info
- Read the **cross-project audit log** (with user attribution, source IP,
  and API-token prefix per event)

## Installing

This provider is not yet published to the Terraform Registry. For local
development, build from source and wire it via `dev_overrides`:

```sh
git clone https://github.com/hatemosphere/terraform-provider-hcloudplus
cd terraform-provider-hcloudplus
make install   # installs to ~/.terraform.d/plugins/... OR use dev_overrides
```

Add a dev override in `~/.terraformrc`:

```hcl
provider_installation {
  dev_overrides {
    "hatemosphere/hcloudplus" = "/path/to/terraform-provider-hcloudplus"
  }
  direct {}
}
```

Then skip `terraform init` for modules that use this provider — run
`terraform plan` / `apply` directly.

## Authentication

Two modes, set via provider block or matching `HCLOUDPLUS_*` env vars.
Either one works; pick based on whether you already have a token.

### Mode A — cold login from robot credentials (fully automated)

The provider launches a headless Chromium internally (via
`hcloud-go-plus/authbrowser`), completes the OAuth + TOTP flow, and mints a
user token on Configure. **Only TOTP-based 2FA is supported** — Yubikey-OTP
accounts cannot be automated. See [hcloud-go-plus 2FA docs](https://github.com/hatemosphere/hcloud-go-plus/blob/main/docs/2fa.md).

Requires an installed Chrome/Chromium on the machine running Terraform
(nothing is downloaded at runtime); in CI containers bake it into the image.

```hcl
provider "hcloudplus" {
  # all optional here — env vars do the same job
}
```

```sh
export HCLOUDPLUS_USERNAME="..."
export HCLOUDPLUS_PASSWORD="..."
export HCLOUDPLUS_TOTP_CMD="op read 'op://vault/accounts.hetzner.com/one-time password?attribute=totp'"
```

### Mode B — bring your own pre-minted user token (faster iteration)

Extract a user token with [`hcgp-login`](https://github.com/hatemosphere/hcloud-go-plus/tree/main/authbrowser/cmd/hcgp-login)
or from the browser cookie. Tokens live ~12h.

```sh
export HCLOUDPLUS_USER_TOKEN="$(hcgp-login)"
```

## Usage

```hcl
terraform {
  required_providers {
    hcloudplus = {
      source = "hatemosphere/hcloudplus"
    }
  }
}

provider "hcloudplus" {}

resource "hcloudplus_project" "infra" {
  name = "infra-prod"
}

resource "hcloudplus_project_token" "pulumi" {
  project_id  = hcloudplus_project.infra.id
  description = "pulumi-rw"
  readonly    = false
}

# Hand the secret to the upstream hcloud provider:
provider "hcloud" {
  token = hcloudplus_project_token.pulumi.secret
}
```

See [`examples/`](./examples/) for complete working configurations.

## Resources

| Resource | Description |
|---|---|
| `hcloudplus_project` | Create/rename/delete account-scoped projects |
| `hcloudplus_project_token` | Create/delete project API tokens; secret is output once |
| `hcloudplus_project_invitation` | Invite + revoke project members; supports `reinvite_on_expiry` |

## Data Sources

| Data source | Description |
|---|---|
| `hcloudplus_user` | Authenticated account info |
| `hcloudplus_limits` | Account-wide resource limits and current usage |
| `hcloudplus_projects` | All projects the account has access to |
| `hcloudplus_activities` | Cross-project audit log with filters |

## Stability

These endpoints are **undocumented** by Hetzner and **not stability-tested**.
Expect breakage when Hetzner updates the console frontend. Patches welcome.

## License

Apache-2.0
