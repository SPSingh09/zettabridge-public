# Staging Terraform root

All `.tf` files for AWS staging live **in this directory**.

```bash
terraform init
terraform validate
terraform plan
```

Do **not** run those commands from the parent `deploy/terraform/` folder — that directory only holds shared docs and `tf.sh`.

See [../README.md](../README.md) for the full deploy guide.

## Phase 4 — execution adapter split

Core and Zerodha adapter run as separate ECS tasks when `execution_adapter_mode = "http"`.
Paper stays cohosted in Core by default (`paper_adapter_cohosted = true`).

| Variable | Day-0 default | Notes |
|----------|---------------|-------|
| `enabled_adapters` | `["paper","zerodha"]` | Angel/Dhan/MT5 ignored until binary + E2E ready |
| `execution_adapter_mode` | `"local"` | Set `"http"` in tfvars to enable sidecars |
| `paper_adapter_cohosted` | `true` | No paper-adapter ECS task |
| `zerodha_adapter_domain_name` | `""` | Required for Kite OAuth (public ALB host rule) |

After apply with http mode:

```bash
terraform output zerodha_adapter_url        # Core ZERODHA_ADAPTER_URL
terraform output execution_adapter_mode
terraform output deployed_sidecar_adapters
```

To add Angel later (when `cmd/angel-adapter` exists):

```hcl
enabled_adapters = ["paper", "zerodha", "angel"]
terraform apply
```
