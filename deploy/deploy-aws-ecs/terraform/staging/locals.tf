locals {
  name_prefix = "${var.project_name}-${var.environment}"

  azs = slice(data.aws_availability_zones.available.names, 0, 2)

  public_subnet_cidrs  = [for i in range(2) : cidrsubnet(var.vpc_cidr, 8, i)]
  private_subnet_cidrs = [for i in range(2) : cidrsubnet(var.vpc_cidr, 8, i + 10)]

  db_name     = "zettabridge"
  db_username = "zettabridge"

  # ── Execution adapters ─────────────────────────────────────────────────────
  enabled_adapters_csv = join(",", sort(tolist(var.enabled_adapters)))

  paper_sidecar_enabled = contains(var.enabled_adapters, "paper") && !var.paper_adapter_cohosted && var.execution_adapter_mode == "http"

  sidecar_adapter_names = var.execution_adapter_mode == "http" ? toset([
    for name in var.enabled_adapters : name if name != "paper" || !var.paper_adapter_cohosted
  ]) : toset([])

  # Only adapters with shipped cmd/*-adapter binaries get ECS tasks today.
  ecs_sidecar_adapters = setintersection(local.sidecar_adapter_names, toset(["paper", "zerodha"]))

  service_discovery_namespace = "${local.name_prefix}.local"

  zerodha_adapter_public_url = var.zerodha_adapter_domain_name != "" ? (
    var.enable_https && local.tls_certificate_arn != "" ?
    "https://${var.zerodha_adapter_domain_name}" :
    "http://${var.zerodha_adapter_domain_name}"
  ) : ""

  zerodha_adapter_url = local.zerodha_adapter_public_url != "" ? local.zerodha_adapter_public_url : (
    contains(local.ecs_sidecar_adapters, "zerodha") ?
    "http://zerodha-adapter.${local.service_discovery_namespace}:8092" : ""
  )

  paper_adapter_url = local.paper_sidecar_enabled ? "http://paper-adapter.${local.service_discovery_namespace}:8091" : ""

  kite_oauth_callback_url = local.zerodha_adapter_url != "" ? "${local.zerodha_adapter_url}/v1/credentials/zerodha/callback" : ""

  kite_publisher_callback_url = local.zerodha_adapter_url != "" ? "${local.zerodha_adapter_url}/v1/publisher/callback" : ""

  adapter_specs = {
    paper = {
      port      = 8091
      entrypoint = ["/app/paper-adapter"]
      health_path = "/v1/healthz"
      public_alb  = false
      enabled_adapters_env = "paper"
    }
    zerodha = {
      port      = 8092
      entrypoint = ["/app/zerodha-adapter"]
      health_path = "/v1/healthz"
      public_alb  = true
      enabled_adapters_env = "zerodha"
    }
  }
}
