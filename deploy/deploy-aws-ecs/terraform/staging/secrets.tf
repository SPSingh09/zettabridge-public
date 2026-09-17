resource "random_password" "db_password" {
  length  = 24
  special = false
}

resource "random_password" "jwt_secret" {
  length  = 48
  special = false
}

resource "random_password" "metrics_token" {
  length  = 32
  special = false
}

resource "random_password" "zb_service_token" {
  length  = 48
  special = false
}

# 32 bytes → 64 hex chars for AES-256-GCM (credenc.ParseKey).
resource "random_id" "aes_key" {
  byte_length = 32
}

resource "aws_secretsmanager_secret" "app" {
  name = "${var.project_name}/${var.environment}/app"

  tags = {
    Name = "${local.name_prefix}-app"
  }
}

resource "aws_secretsmanager_secret_version" "app" {
  secret_id = aws_secretsmanager_secret.app.id
  secret_string = jsonencode(merge(
    {
      JWT_SECRET    = random_password.jwt_secret.result
      AES_KEY       = random_id.aes_key.hex
      DATABASE_URL  = "postgres://${local.db_username}:${random_password.db_password.result}@${aws_db_instance.main.address}:5432/${local.db_name}?sslmode=require"
      METRICS_TOKEN = random_password.metrics_token.result
      ZB_SERVICE_TOKEN = random_password.zb_service_token.result
    },
    var.stripe_secret_key != "" ? {
      STRIPE_SECRET_KEY     = var.stripe_secret_key
      STRIPE_WEBHOOK_SECRET = var.stripe_webhook_secret
    } : {},
    var.zerodha_api_key != "" ? {
      ZERODHA_API_KEY    = var.zerodha_api_key
      ZERODHA_API_SECRET = var.zerodha_api_secret
    } : {},
    var.fyers_app_id != "" ? {
      FYERS_APP_ID    = var.fyers_app_id
      FYERS_SECRET_ID = var.fyers_secret_id
    } : {},
    var.smtp_user != "" ? {
      SMTP_USER = var.smtp_user
      SMTP_PASS = var.smtp_pass
    } : {},
    var.telegram_bot_token != "" ? {
      TELEGRAM_BOT_TOKEN = var.telegram_bot_token
    } : {}
  ))

  depends_on = [aws_db_instance.main]
}
