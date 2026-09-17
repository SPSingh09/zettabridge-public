variable "aws_region" {
  description = "AWS region (ap-south-1 recommended for Indian brokers)."
  type        = string
  default     = "ap-south-1"
}

variable "project_name" {
  description = "Resource name prefix."
  type        = string
  default     = "zettabridge"
}

variable "environment" {
  description = "Environment label (staging only in this scaffold)."
  type        = string
  default     = "staging"
}

variable "vpc_cidr" {
  description = "VPC CIDR block."
  type        = string
  default     = "10.0.0.0/16"
}

variable "enable_https" {
  description = "Enable HTTPS listener on ALB (requires domain or acm_certificate_arn)."
  type        = bool
  default     = false
}

variable "api_domain_name" {
  description = "API hostname for ACM cert and APP_PUBLIC_URL (e.g. api.staging.example.com)."
  type        = string
  default     = ""
}

variable "route53_zone_id" {
  description = "Route53 hosted zone ID for ACM DNS validation (required when enable_https and no acm_certificate_arn)."
  type        = string
  default     = ""
}

variable "acm_certificate_arn" {
  description = "Existing ACM certificate ARN; skips ACM creation when set."
  type        = string
  default     = ""
}

variable "cors_origins" {
  description = "CORS_ORIGINS env value."
  type        = string
  default     = "*"
}

variable "bootstrap_admin_email" {
  description = "BOOTSTRAP_ADMIN_EMAIL — kept active so DB wipes auto-recreate the admin."
  type        = string
  default     = ""
}

variable "bootstrap_admin_password" {
  description = "BOOTSTRAP_ADMIN_PASSWORD — used alongside bootstrap_admin_email to create admin if not exists."
  type        = string
  default     = ""
  sensitive   = true
}

variable "broker_mode" {
  description = "BROKER_MODE: mock for staging smoke; live after broker IP whitelist."
  type        = string
  default     = "mock"

  validation {
    condition     = contains(["mock", "live"], var.broker_mode)
    error_message = "broker_mode must be mock or live."
  }
}

variable "email_verification_required" {
  description = "EMAIL_VERIFICATION_REQUIRED — use false for early staging smoke."
  type        = bool
  default     = false
}

variable "zerodha_publisher_enabled" {
  description = "ZERODHA_PUBLISHER_ENABLED — enables Kite Publisher credential creation and order routing."
  type        = bool
  default     = false
}

variable "zerodha_publisher_callback_url" {
  description = "ZERODHA_PUBLISHER_CALLBACK_URL — Kite redirect target after basket confirmation (e.g. https://api.staging.zettabridge.net/v1/publisher/callback). Falls back to APP_PUBLIC_URL/v1/publisher/callback if empty."
  type        = string
  default     = ""
}

variable "email_provider" {
  description = "EMAIL_PROVIDER: log | smtp."
  type        = string
  default     = "log"
}

variable "email_from" {
  description = "EMAIL_FROM — display name + address shown in From header (e.g. 'ZettaBridge <noreply@zettabridge.in>')."
  type        = string
  default     = "noreply@localhost"
}

variable "smtp_host" {
  description = "SMTP_HOST (e.g. smtp.zoho.in for Zoho Indian data-center, smtp.sendgrid.net for SendGrid)."
  type        = string
  default     = ""
}

variable "smtp_port" {
  description = "SMTP_PORT — 587 (STARTTLS, recommended) or 465 (implicit SSL, requires library change)."
  type        = number
  default     = 587
}

variable "smtp_user" {
  description = "SMTP_USER — full mailbox address for Zoho (e.g. noreply@zettabridge.in); 'apikey' for SendGrid."
  type        = string
  default     = ""
  sensitive   = true
}

variable "smtp_pass" {
  description = "SMTP_PASS — Zoho App Password (generate at Zoho Account → Security → App Passwords) or SendGrid API key."
  type        = string
  default     = ""
  sensitive   = true
}

variable "sebi_algo_id_required" {
  description = "SEBI_ALGO_ID_REQUIRED — keep false until algo IDs configured."
  type        = bool
  default     = false
}

variable "ecs_desired_count" {
  description = "Number of Fargate tasks (set 0 to pause compute, NAT/RDS still bill)."
  type        = number
  default     = 1
}

variable "ecs_cpu" {
  description = "Fargate CPU units (512 = 0.5 vCPU)."
  type        = number
  default     = 512
}

variable "ecs_memory" {
  description = "Fargate memory MiB."
  type        = number
  default     = 1024
}

variable "image_tag" {
  description = "ECR image tag to deploy."
  type        = string
  default     = "staging"
}

variable "db_instance_class" {
  description = "RDS instance class (smallest Graviton)."
  type        = string
  default     = "db.t4g.micro"
}

variable "db_allocated_storage_gb" {
  description = "RDS gp3 storage GiB."
  type        = number
  default     = 20
}

variable "cache_node_type" {
  description = "ElastiCache Redis node type."
  type        = string
  default     = "cache.t4g.micro"
}

variable "log_retention_days" {
  description = "CloudWatch log retention for ECS."
  type        = number
  default     = 14
}

variable "alarm_email" {
  description = "Email address for CloudWatch alarm SNS notifications. Leave empty to suppress email delivery (alarms still visible in AWS Console)."
  type        = string
  default     = ""
}

variable "telegram_bot_token" {
  description = "Telegram Bot API token (from @BotFather). Used to send per-trade alerts to users who connect their chat ID. Leave empty to disable Telegram alerts."
  type        = string
  default     = ""
  sensitive   = true
}

variable "ecr_image_count_to_keep" {
  description = "ECR lifecycle — max images to retain."
  type        = number
  default     = 5
}

# ── Dashboard ─────────────────────────────────────────────────────────────────

variable "dashboard_image_tag" {
  description = "ECR image tag for the dashboard container."
  type        = string
  default     = "staging"
}

variable "app_domain_name" {
  description = "Dashboard hostname for CORS and NEXT_PUBLIC_APP_URL (e.g. app.staging.example.com)."
  type        = string
  default     = ""
}

variable "dashboard_desired_count" {
  description = "Number of dashboard Fargate tasks (set 0 to pause)."
  type        = number
  default     = 1
}

# ── Stripe billing ────────────────────────────────────────────────────────────

variable "billing_enabled" {
  description = "Enable Stripe billing (set false for initial smoke test)."
  type        = bool
  default     = false
}

variable "stripe_secret_key" {
  description = "Stripe secret key (sk_live_* or sk_test_*). Stored in Secrets Manager."
  type        = string
  default     = ""
  sensitive   = true
}

variable "stripe_webhook_secret" {
  description = "Stripe webhook signing secret (whsec_*). Stored in Secrets Manager."
  type        = string
  default     = ""
  sensitive   = true
}

variable "stripe_price_pro" {
  description = "Stripe Price ID for Pro plan ($29/mo)."
  type        = string
  default     = ""
}

# ── Zerodha Kite Connect OAuth ────────────────────────────────────────────────

variable "zerodha_api_key" {
  description = "Kite Connect API key (from developers.kite.trade). Stored in Secrets Manager."
  type        = string
  default     = ""
  sensitive   = true
}

variable "zerodha_api_secret" {
  description = "Kite Connect API secret. Stored in Secrets Manager."
  type        = string
  default     = ""
  sensitive   = true
}

variable "zerodha_callback_url" {
  description = "Kite OAuth redirect URL — must match the URL set in the Kite app."
  type        = string
  default     = ""
}

variable "zerodha_frontend_url" {
  description = "Frontend base URL to redirect users after OAuth (e.g. https://app.staging.zettabridge.net)."
  type        = string
  default     = ""
}

# ── FYERS market-data OAuth (Paper Trading mark-to-market, Phase 6) ───────────
# Single platform-wide connection, admin-connected via the dashboard —
# never used for order placement, unrelated to the per-user broker
# credentials above.

variable "fyers_app_id" {
  description = "FYERS App ID (client_id) — from myapi.fyers.in/dashboard. Stored in Secrets Manager."
  type        = string
  default     = ""
  sensitive   = true
}

variable "fyers_secret_id" {
  description = "FYERS Secret ID. Stored in Secrets Manager."
  type        = string
  default     = ""
  sensitive   = true
}

variable "fyers_callback_url" {
  description = "FYERS OAuth redirect URL — must match the redirect URI registered in the FYERS app dashboard exactly (e.g. https://api.staging.zettabridge.net/v1/admin/fyers/callback)."
  type        = string
  default     = ""
}

# ── Execution adapters (Phase 4 split deploy) ────────────────────────────────

variable "enabled_adapters" {
  description = "Execution adapters enabled in Core (ENABLED_ADAPTERS). Day 0: paper + zerodha only."
  type        = set(string)
  default     = ["paper", "zerodha"]

  validation {
    condition = length(setsubtract(var.enabled_adapters, toset([
      "paper", "zerodha", "angel", "dhan", "mt5", "fyers",
    ]))) == 0
    error_message = "enabled_adapters must be a subset of: paper, zerodha, angel, dhan, mt5, fyers."
  }
}

variable "execution_adapter_mode" {
  description = "Core EXECUTION_ADAPTER_MODE: local (in-process) or http (sidecar ECS services)."
  type        = string
  default     = "local"

  validation {
    condition     = contains(["local", "http"], var.execution_adapter_mode)
    error_message = "execution_adapter_mode must be local or http."
  }
}

variable "paper_adapter_cohosted" {
  description = "When true, paper execution stays in the Core task (no paper-adapter ECS service)."
  type        = bool
  default     = true
}

variable "zerodha_adapter_domain_name" {
  description = "Public hostname for Zerodha adapter (Kite OAuth + Publisher callbacks). e.g. exec-zerodha.staging.zettabridge.net — add CNAME in Cloudflare and include as ACM SAN when using HTTPS."
  type        = string
  default     = ""
}

variable "adapter_ecs_cpu" {
  description = "Fargate CPU units per adapter sidecar (256 = 0.25 vCPU)."
  type        = number
  default     = 256
}

variable "adapter_ecs_memory" {
  description = "Fargate memory MiB per adapter sidecar."
  type        = number
  default     = 512
}

variable "adapter_desired_count" {
  description = "Desired task count per deployed adapter sidecar (0 to pause)."
  type        = number
  default     = 1
}
