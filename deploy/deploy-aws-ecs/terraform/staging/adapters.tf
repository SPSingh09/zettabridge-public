# Phase 4 — conditional execution adapter sidecars (paper + zerodha day 0).

resource "aws_service_discovery_private_dns_namespace" "adapters" {
  count = length(local.ecs_sidecar_adapters) > 0 ? 1 : 0

  name        = local.service_discovery_namespace
  description = "Private DNS for Core → adapter HTTP calls"
  vpc         = aws_vpc.main.id

  tags = {
    Name = local.name_prefix
  }
}

resource "aws_service_discovery_service" "adapter" {
  for_each = local.ecs_sidecar_adapters

  name = "${each.key}-adapter"

  dns_config {
    namespace_id = aws_service_discovery_private_dns_namespace.adapters[0].id

    dns_records {
      ttl  = 10
      type = "A"
    }

    routing_policy = "MULTIVALUE"
  }

  health_check_custom_config {
    failure_threshold = 1
  }

  tags = {
    Name = "${local.name_prefix}-${each.key}-adapter"
  }
}

locals {
  adapter_base_environment = [
    { name = "APP_ENV", value = "staging" },
    { name = "REDIS_URL", value = "redis://${aws_elasticache_cluster.main.cache_nodes[0].address}:6379" },
    { name = "BROKER_MODE", value = var.broker_mode },
    { name = "BROKER_HTTP_TIMEOUT_SEC", value = "8" },
    { name = "BROKER_HTTP_GET_RETRIES", value = "2" },
    { name = "CORE_INTERNAL_URL", value = local.api_url },
  ]

  adapter_base_secrets = [
    { name = "JWT_SECRET", valueFrom = "${aws_secretsmanager_secret.app.arn}:JWT_SECRET::" },
    { name = "AES_KEY", valueFrom = "${aws_secretsmanager_secret.app.arn}:AES_KEY::" },
    { name = "DATABASE_URL", valueFrom = "${aws_secretsmanager_secret.app.arn}:DATABASE_URL::" },
    { name = "ZB_SERVICE_TOKEN", valueFrom = "${aws_secretsmanager_secret.app.arn}:ZB_SERVICE_TOKEN::" },
  ]
}

resource "aws_ecs_task_definition" "adapter" {
  for_each = local.ecs_sidecar_adapters

  family                   = "${local.name_prefix}-${each.key}-adapter"
  requires_compatibilities = ["FARGATE"]
  network_mode             = "awsvpc"
  cpu                      = var.adapter_ecs_cpu
  memory                   = var.adapter_ecs_memory
  execution_role_arn       = aws_iam_role.ecs_execution.arn
  task_role_arn            = aws_iam_role.ecs_task.arn

  container_definitions = jsonencode([{
    name       = "${each.key}-adapter"
    image      = "${aws_ecr_repository.app.repository_url}:${var.image_tag}"
    essential  = true
    entryPoint = local.adapter_specs[each.key].entrypoint

    portMappings = [{
      containerPort = local.adapter_specs[each.key].port
      hostPort      = local.adapter_specs[each.key].port
      protocol      = "tcp"
    }]

    environment = concat(
      local.adapter_base_environment,
      [
        { name = "ADAPTER_PORT", value = tostring(local.adapter_specs[each.key].port) },
        { name = "ENABLED_ADAPTERS", value = local.adapter_specs[each.key].enabled_adapters_env },
      ],
      each.key == "zerodha" ? [
        { name = "ADAPTER_PUBLIC_URL", value = local.zerodha_adapter_url },
        { name = "FRONTEND_URL", value = var.zerodha_frontend_url != "" ? var.zerodha_frontend_url : local.app_url },
        { name = "ZERODHA_FRONTEND_URL", value = var.zerodha_frontend_url != "" ? var.zerodha_frontend_url : local.app_url },
      ] : [],
      each.key == "zerodha" && var.zerodha_publisher_enabled ? [
        { name = "ZERODHA_PUBLISHER_ENABLED", value = "true" },
        { name = "ZERODHA_PUBLISHER_CALLBACK_URL", value = local.kite_publisher_callback_url },
      ] : [],
      each.key == "zerodha" ? [
        { name = "BROKER_ANGEL_CLIENT_LOCAL_IP", value = aws_eip.nat.public_ip },
        { name = "BROKER_ANGEL_CLIENT_PUBLIC_IP", value = aws_eip.nat.public_ip },
        { name = "BROKER_ANGEL_MAC_ADDRESS", value = "00:00:00:00:00:00" },
      ] : [],
    )

    secrets = concat(
      local.adapter_base_secrets,
      each.key == "zerodha" && var.zerodha_api_key != "" ? [
        { name = "ZERODHA_API_KEY", valueFrom = "${aws_secretsmanager_secret.app.arn}:ZERODHA_API_KEY::" },
        { name = "ZERODHA_API_SECRET", valueFrom = "${aws_secretsmanager_secret.app.arn}:ZERODHA_API_SECRET::" },
      ] : [],
    )

    healthCheck = {
      command     = ["CMD-SHELL", "wget -qO- http://localhost:${local.adapter_specs[each.key].port}${local.adapter_specs[each.key].health_path} || exit 1"]
      interval    = 30
      timeout     = 5
      retries     = 3
      startPeriod = 15
    }

    logConfiguration = {
      logDriver = "awslogs"
      options = {
        awslogs-group         = aws_cloudwatch_log_group.ecs.name
        awslogs-region        = var.aws_region
        awslogs-stream-prefix = "${each.key}-adapter"
      }
    }
  }])

  tags = {
    Name = "${local.name_prefix}-${each.key}-adapter"
  }
}

resource "aws_ecs_service" "adapter" {
  for_each = local.ecs_sidecar_adapters

  name            = "${local.name_prefix}-${each.key}-adapter"
  cluster         = aws_ecs_cluster.main.id
  task_definition = aws_ecs_task_definition.adapter[each.key].arn
  desired_count   = var.adapter_desired_count
  launch_type     = "FARGATE"

  network_configuration {
    subnets          = aws_subnet.private[*].id
    security_groups  = [aws_security_group.ecs.id]
    assign_public_ip = false
  }

  service_registries {
    registry_arn = aws_service_discovery_service.adapter[each.key].arn
  }

  dynamic "load_balancer" {
    for_each = each.key == "zerodha" && var.zerodha_adapter_domain_name != "" ? [1] : []
    content {
      target_group_arn = aws_lb_target_group.zerodha_adapter[0].arn
      container_name   = "zerodha-adapter"
      container_port   = 8092
    }
  }

  # Rolling deploy — see aws_ecs_service.app for reasoning.
  deployment_minimum_healthy_percent = 100
  deployment_maximum_percent         = 200
  health_check_grace_period_seconds  = 60

  force_new_deployment = true
  triggers = {
    task_definition_revision = aws_ecs_task_definition.adapter[each.key].revision
  }

  depends_on = [
    aws_lb_listener.http_forward,
    aws_lb_listener.http_redirect,
    aws_lb_listener.https,
    aws_secretsmanager_secret_version.app,
  ]

  tags = {
    Name = "${local.name_prefix}-${each.key}-adapter"
  }
}
