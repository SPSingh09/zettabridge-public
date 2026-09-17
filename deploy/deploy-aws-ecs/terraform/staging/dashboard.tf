resource "aws_ecr_repository" "dashboard" {
  name                 = "${local.name_prefix}-dashboard"
  image_tag_mutability = "MUTABLE"
  force_delete         = true

  image_scanning_configuration {
    scan_on_push = true
  }

  tags = {
    Name = "${local.name_prefix}-dashboard"
  }
}

resource "aws_ecr_lifecycle_policy" "dashboard" {
  repository = aws_ecr_repository.dashboard.name

  policy = jsonencode({
    rules = [{
      rulePriority = 1
      description  = "Keep last N images"
      selection = {
        tagStatus   = "any"
        countType   = "imageCountMoreThan"
        countNumber = var.ecr_image_count_to_keep
      }
      action = {
        type = "expire"
      }
    }]
  })
}

resource "aws_ecs_task_definition" "dashboard" {
  family                   = "${local.name_prefix}-dashboard"
  requires_compatibilities = ["FARGATE"]
  network_mode             = "awsvpc"
  cpu                      = 256
  memory                   = 512
  execution_role_arn       = aws_iam_role.ecs_execution.arn
  task_role_arn            = aws_iam_role.ecs_task.arn

  container_definitions = jsonencode([{
    name      = "dashboard"
    image     = "${aws_ecr_repository.dashboard.repository_url}:${var.dashboard_image_tag}"
    essential = true

    portMappings = [{
      containerPort = 3000
      hostPort      = 3000
      protocol      = "tcp"
    }]

    environment = [
      { name = "NEXT_PUBLIC_API_URL", value = local.api_url },
      { name = "NEXT_PUBLIC_APP_URL", value = local.app_url },
      { name = "NEXT_PUBLIC_BILLING_ENABLED", value = var.billing_enabled ? "true" : "false" },
      { name = "PORT", value = "3000" },
      { name = "HOSTNAME", value = "0.0.0.0" },
    ]

    healthCheck = {
      command     = ["CMD-SHELL", "wget -q -O /dev/null http://localhost:3000/ || exit 1"]
      interval    = 30
      timeout     = 5
      retries     = 3
      startPeriod = 10
    }

    logConfiguration = {
      logDriver = "awslogs"
      options = {
        awslogs-group         = aws_cloudwatch_log_group.ecs.name
        awslogs-region        = var.aws_region
        awslogs-stream-prefix = "dashboard"
      }
    }
  }])

  tags = {
    Name = "${local.name_prefix}-dashboard"
  }
}

# Dashboard target group on port 3000
resource "aws_lb_target_group" "dashboard" {
  name        = "${local.name_prefix}-dashboard-tg"
  port        = 3000
  protocol    = "HTTP"
  vpc_id      = aws_vpc.main.id
  target_type = "ip"

  health_check {
    enabled             = true
    path                = "/login"
    matcher             = "200"
    healthy_threshold   = 2
    unhealthy_threshold = 3
    interval            = 30
    timeout             = 5
  }

  tags = {
    Name = "${local.name_prefix}-dashboard"
  }
}

resource "aws_ecs_service" "dashboard" {
  name            = "${local.name_prefix}-dashboard"
  cluster         = aws_ecs_cluster.main.id
  task_definition = aws_ecs_task_definition.dashboard.arn
  desired_count   = var.dashboard_desired_count
  launch_type     = "FARGATE"

  network_configuration {
    subnets          = aws_subnet.private[*].id
    security_groups  = [aws_security_group.ecs.id]
    assign_public_ip = false
  }

  # Only attach to ALB when a listener rule exists (i.e. app_domain_name is set).
  # Phase A (no domain): service runs without LB — backend smoke test works fine.
  # Phase B (domain set): terraform apply recreates the service with LB attachment.
  dynamic "load_balancer" {
    for_each = var.app_domain_name != "" ? [1] : []
    content {
      target_group_arn = aws_lb_target_group.dashboard.arn
      container_name   = "dashboard"
      container_port   = 3000
    }
  }

  deployment_minimum_healthy_percent = 100
  deployment_maximum_percent         = 200

  depends_on = [
    aws_lb_listener.http_forward,
    aws_lb_listener.http_redirect,
    aws_lb_listener.https,
  ]

  tags = {
    Name = "${local.name_prefix}-dashboard"
  }
}
