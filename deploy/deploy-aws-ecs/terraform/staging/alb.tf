resource "aws_lb" "main" {
  name               = local.name_prefix
  internal           = false
  load_balancer_type = "application"
  security_groups    = [aws_security_group.alb.id]
  subnets            = aws_subnet.public[*].id

  tags = {
    Name = local.name_prefix
  }
}

resource "aws_lb_target_group" "app" {
  name        = "${local.name_prefix}-tg"
  port        = 8080
  protocol    = "HTTP"
  vpc_id      = aws_vpc.main.id
  target_type = "ip"

  # AWS defaults to 300s here. With recreate-style deploys (see
  # aws_ecs_service.app's deployment_minimum_healthy_percent=0), ECS waits
  # out this drain before it finishes stopping the old task — on a
  # single-instance staging service with no real concurrent user traffic to
  # protect, a short window is enough and keeps deploy downtime predictable.
  deregistration_delay = 10

  health_check {
    enabled             = true
    path                = "/healthz"
    matcher             = "200"
    healthy_threshold   = 2
    unhealthy_threshold = 3
    interval            = 30
    timeout             = 5
  }

  tags = {
    Name = local.name_prefix
  }
}

# Zerodha execution adapter — public ALB for Kite OAuth + Publisher callbacks.
resource "aws_lb_target_group" "zerodha_adapter" {
  count = contains(local.ecs_sidecar_adapters, "zerodha") && var.zerodha_adapter_domain_name != "" ? 1 : 0

  name        = "${local.name_prefix}-zerodha-tg"
  port        = 8092
  protocol    = "HTTP"
  vpc_id      = aws_vpc.main.id
  target_type = "ip"

  # See aws_lb_target_group.app — same reasoning.
  deregistration_delay = 10

  health_check {
    enabled             = true
    path                = "/v1/healthz"
    matcher             = "200"
    healthy_threshold   = 2
    unhealthy_threshold = 3
    interval            = 30
    timeout             = 5
  }

  tags = {
    Name = "${local.name_prefix}-zerodha-adapter"
  }
}

resource "aws_lb_listener" "http_forward" {
  count = var.enable_https && local.tls_certificate_arn != "" ? 0 : 1

  load_balancer_arn = aws_lb.main.arn
  port              = 80
  protocol          = "HTTP"

  default_action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.app.arn
  }
}

resource "aws_lb_listener" "http_redirect" {
  count = var.enable_https && local.tls_certificate_arn != "" ? 1 : 0

  load_balancer_arn = aws_lb.main.arn
  port              = 80
  protocol          = "HTTP"

  default_action {
    type = "redirect"
    redirect {
      port        = "443"
      protocol    = "HTTPS"
      status_code = "HTTP_301"
    }
  }
}

resource "aws_lb_listener" "https" {
  count = var.enable_https && local.tls_certificate_arn != "" ? 1 : 0

  load_balancer_arn = aws_lb.main.arn
  port              = 443
  protocol          = "HTTPS"
  ssl_policy        = "ELBSecurityPolicy-TLS13-1-2-2021-06"
  certificate_arn   = local.tls_certificate_arn

  default_action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.app.arn
  }
}

# Route dashboard traffic by host header when app_domain_name is set.
# Without a domain (first HTTP-only apply), the dashboard TG exists but is unreachable
# via ALB — backend smoke test still works; dashboard becomes routable after Cloudflare
# CNAMEs are added and app_domain_name is set in tfvars.
resource "aws_lb_listener_rule" "dashboard_http" {
  count        = length(aws_lb_listener.http_forward) > 0 && var.app_domain_name != "" ? 1 : 0
  listener_arn = aws_lb_listener.http_forward[0].arn
  priority     = 10

  action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.dashboard.arn
  }

  condition {
    host_header {
      values = [var.app_domain_name]
    }
  }
}

resource "aws_lb_listener_rule" "dashboard_https" {
  count        = length(aws_lb_listener.https) > 0 && var.app_domain_name != "" ? 1 : 0
  listener_arn = aws_lb_listener.https[0].arn
  priority     = 10

  action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.dashboard.arn
  }

  condition {
    host_header {
      values = [var.app_domain_name]
    }
  }
}

resource "aws_lb_listener_rule" "zerodha_adapter_http" {
  count        = length(aws_lb_listener.http_forward) > 0 && length(aws_lb_target_group.zerodha_adapter) > 0 ? 1 : 0
  listener_arn = aws_lb_listener.http_forward[0].arn
  priority     = 20

  action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.zerodha_adapter[0].arn
  }

  condition {
    host_header {
      values = [var.zerodha_adapter_domain_name]
    }
  }
}

resource "aws_lb_listener_rule" "zerodha_adapter_https" {
  count        = length(aws_lb_listener.https) > 0 && length(aws_lb_target_group.zerodha_adapter) > 0 ? 1 : 0
  listener_arn = aws_lb_listener.https[0].arn
  priority     = 20

  action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.zerodha_adapter[0].arn
  }

  condition {
    host_header {
      values = [var.zerodha_adapter_domain_name]
    }
  }
}

locals {
  api_url = var.enable_https && var.api_domain_name != "" ? "https://${var.api_domain_name}" : "http://${aws_lb.main.dns_name}"
  app_url = var.enable_https && var.app_domain_name != "" ? "https://${var.app_domain_name}" : "http://${aws_lb.main.dns_name}"
}
