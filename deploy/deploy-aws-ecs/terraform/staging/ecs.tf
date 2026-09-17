resource "aws_ecs_cluster" "main" {
  name = local.name_prefix

  setting {
    name  = "containerInsights"
    value = "disabled"
  }

  tags = {
    Name = local.name_prefix
  }
}

locals {
  container_environment = concat(
    [
      { name = "APP_ENV", value = "staging" },
      { name = "PORT", value = "8080" },
      { name = "REDIS_URL", value = "redis://${aws_elasticache_cluster.main.cache_nodes[0].address}:6379" },
      { name = "CORS_ORIGINS", value = var.cors_origins },
      { name = "APP_PUBLIC_URL", value = local.api_url },
      { name = "FRONTEND_URL", value = local.app_url },
      { name = "WORKER_COUNT", value = "20" },
      { name = "QUEUE_BUFFER", value = "1000" },
      { name = "BROKER_MODE", value = var.broker_mode },
      { name = "ENABLED_ADAPTERS", value = local.enabled_adapters_csv },
      { name = "EXECUTION_ADAPTER_MODE", value = var.execution_adapter_mode },
      { name = "BROKER_HTTP_TIMEOUT_SEC", value = "8" },
      { name = "BROKER_HTTP_GET_RETRIES", value = "2" },
      { name = "BROKER_ANGEL_CLIENT_LOCAL_IP", value = aws_eip.nat.public_ip },
      { name = "BROKER_ANGEL_CLIENT_PUBLIC_IP", value = aws_eip.nat.public_ip },
      { name = "BROKER_ANGEL_MAC_ADDRESS", value = "00:00:00:00:00:00" },
      { name = "METRICS_ENABLED", value = "true" },
      { name = "EMAIL_VERIFICATION_REQUIRED", value = var.email_verification_required ? "true" : "false" },
      { name = "EMAIL_PROVIDER", value = var.email_provider },
      { name = "EMAIL_FROM", value = var.email_from },
      { name = "SMTP_HOST", value = var.smtp_host },
      { name = "SMTP_PORT", value = tostring(var.smtp_port) },
      { name = "SEBI_ALGO_ID_REQUIRED", value = var.sebi_algo_id_required ? "true" : "false" },
    ],
    var.bootstrap_admin_email != "" ? [{ name = "BOOTSTRAP_ADMIN_EMAIL", value = var.bootstrap_admin_email }] : [],
    var.bootstrap_admin_password != "" ? [{ name = "BOOTSTRAP_ADMIN_PASSWORD", value = var.bootstrap_admin_password }] : [],
    var.execution_adapter_mode == "http" && local.zerodha_adapter_url != "" ? [
      { name = "ZERODHA_ADAPTER_URL", value = local.zerodha_adapter_url },
    ] : [],
    var.execution_adapter_mode == "http" && local.paper_adapter_url != "" ? [
      { name = "PAPER_ADAPTER_URL", value = local.paper_adapter_url },
    ] : [],
    var.zerodha_api_key != "" && var.execution_adapter_mode != "http" ? [
      { name = "ZERODHA_CALLBACK_URL", value = var.zerodha_callback_url },
      { name = "ZERODHA_FRONTEND_URL", value = var.zerodha_frontend_url },
    ] : [],
    var.zerodha_api_key != "" && var.execution_adapter_mode == "http" ? [
      { name = "ZERODHA_FRONTEND_URL", value = var.zerodha_frontend_url != "" ? var.zerodha_frontend_url : local.app_url },
    ] : [],
    var.zerodha_publisher_enabled && var.execution_adapter_mode != "http" ? [
      { name = "ZERODHA_PUBLISHER_ENABLED", value = "true" },
      { name = "ZERODHA_PUBLISHER_CALLBACK_URL", value = var.zerodha_publisher_callback_url != "" ? var.zerodha_publisher_callback_url : "${local.api_url}/v1/publisher/callback" },
    ] : [],
    var.zerodha_publisher_enabled && var.execution_adapter_mode == "http" ? [
      { name = "ZERODHA_PUBLISHER_ENABLED", value = "true" },
    ] : [],
    var.fyers_app_id != "" ? [
      { name = "FYERS_CALLBACK_URL", value = var.fyers_callback_url != "" ? var.fyers_callback_url : "${local.api_url}/v1/admin/fyers/callback" },
    ] : [],
  )

  container_secrets = concat(
    [
      { name = "JWT_SECRET", valueFrom = "${aws_secretsmanager_secret.app.arn}:JWT_SECRET::" },
      { name = "AES_KEY", valueFrom = "${aws_secretsmanager_secret.app.arn}:AES_KEY::" },
      { name = "DATABASE_URL", valueFrom = "${aws_secretsmanager_secret.app.arn}:DATABASE_URL::" },
      { name = "METRICS_TOKEN", valueFrom = "${aws_secretsmanager_secret.app.arn}:METRICS_TOKEN::" },
    ],
    var.execution_adapter_mode == "http" ? [
      { name = "ZB_SERVICE_TOKEN", valueFrom = "${aws_secretsmanager_secret.app.arn}:ZB_SERVICE_TOKEN::" },
    ] : [],
    var.stripe_secret_key != "" ? [
      { name = "STRIPE_SECRET_KEY", valueFrom = "${aws_secretsmanager_secret.app.arn}:STRIPE_SECRET_KEY::" },
      { name = "STRIPE_WEBHOOK_SECRET", valueFrom = "${aws_secretsmanager_secret.app.arn}:STRIPE_WEBHOOK_SECRET::" },
    ] : [],
    var.zerodha_api_key != "" ? [
      { name = "ZERODHA_API_KEY", valueFrom = "${aws_secretsmanager_secret.app.arn}:ZERODHA_API_KEY::" },
      { name = "ZERODHA_API_SECRET", valueFrom = "${aws_secretsmanager_secret.app.arn}:ZERODHA_API_SECRET::" },
    ] : [],
    var.fyers_app_id != "" ? [
      { name = "FYERS_APP_ID", valueFrom = "${aws_secretsmanager_secret.app.arn}:FYERS_APP_ID::" },
      { name = "FYERS_SECRET_ID", valueFrom = "${aws_secretsmanager_secret.app.arn}:FYERS_SECRET_ID::" },
    ] : [],
    var.smtp_user != "" ? [
      { name = "SMTP_USER", valueFrom = "${aws_secretsmanager_secret.app.arn}:SMTP_USER::" },
      { name = "SMTP_PASS", valueFrom = "${aws_secretsmanager_secret.app.arn}:SMTP_PASS::" },
    ] : [],
    var.telegram_bot_token != "" ? [
      { name = "TELEGRAM_BOT_TOKEN", valueFrom = "${aws_secretsmanager_secret.app.arn}:TELEGRAM_BOT_TOKEN::" },
    ] : []
  )

  billing_env = var.billing_enabled ? [
    { name = "BILLING_ENABLED", value = "true" },
    { name = "STRIPE_PRICE_PRO", value = var.stripe_price_pro },
    { name = "BILLING_SUCCESS_URL", value = "${local.app_url}/billing/success?session_id={CHECKOUT_SESSION_ID}" },
    { name = "BILLING_CANCEL_URL", value = "${local.app_url}/billing/cancel" },
    { name = "BILLING_PORTAL_RETURN_URL", value = "${local.app_url}/billing" },
  ] : [{ name = "BILLING_ENABLED", value = "false" }]
}

resource "aws_ecs_task_definition" "app" {
  family                   = local.name_prefix
  requires_compatibilities = ["FARGATE"]
  network_mode             = "awsvpc"
  cpu                      = var.ecs_cpu
  memory                   = var.ecs_memory
  execution_role_arn       = aws_iam_role.ecs_execution.arn
  task_role_arn            = aws_iam_role.ecs_task.arn

  container_definitions = jsonencode([{
    name      = "zettabridge"
    image     = "${aws_ecr_repository.app.repository_url}:${var.image_tag}"
    essential = true

    portMappings = [{
      containerPort = 8080
      hostPort      = 8080
      protocol      = "tcp"
    }]

    environment = concat(local.container_environment, local.billing_env)
    secrets     = local.container_secrets

    healthCheck = {
      command     = ["CMD-SHELL", "wget -qO- http://localhost:8080/healthz || exit 1"]
      interval    = 30
      timeout     = 5
      retries     = 3
      startPeriod = 90
    }

    logConfiguration = {
      logDriver = "awslogs"
      options = {
        awslogs-group         = aws_cloudwatch_log_group.ecs.name
        awslogs-region        = var.aws_region
        awslogs-stream-prefix = "ecs"
      }
    }
  }])

  tags = {
    Name = local.name_prefix
  }
}

resource "aws_ecs_service" "app" {
  name            = local.name_prefix
  cluster         = aws_ecs_cluster.main.id
  task_definition = aws_ecs_task_definition.app.arn
  desired_count   = var.ecs_desired_count
  launch_type     = "FARGATE"

  network_configuration {
    subnets          = aws_subnet.private[*].id
    security_groups  = [aws_security_group.ecs.id]
    assign_public_ip = false
  }

  load_balancer {
    target_group_arn = aws_lb_target_group.app.arn
    container_name   = "zettabridge"
    container_port   = 8080
  }

  # Rolling deploy (start new, wait healthy, then stop old): the old+new
  # overlap is short (bounded by the target group's health check settling
  # time — interval=30s, healthy_threshold=2, so ~1-2 min) and, unlike a
  # recreate strategy, a new task that fails to come up leaves the old one
  # running instead of taking the service fully down. The brief overlap can
  # double-fire in-process background jobs (e.g. the market-data sweep
  # ticker) for that ~1-2 min window — fixed at the source by batching the
  # sweep's provider calls (see marketdata.SnapshotJob.Sweep) rather than by
  # forcing downtime on every deploy.
  deployment_minimum_healthy_percent = 100
  deployment_maximum_percent         = 200
  # Core runs migrations + queue wiring before /healthz; ALB checks need grace time.
  health_check_grace_period_seconds = 120

  # Re-roll tasks when Terraform registers a new task definition revision (env/secrets).
  force_new_deployment = true
  triggers = {
    task_definition_revision = aws_ecs_task_definition.app.revision
  }

  depends_on = [
    aws_lb_listener.http_forward,
    aws_lb_listener.http_redirect,
    aws_lb_listener.https,
    aws_secretsmanager_secret_version.app,
    aws_ecs_service.adapter,
  ]

  tags = {
    Name = local.name_prefix
  }
}
