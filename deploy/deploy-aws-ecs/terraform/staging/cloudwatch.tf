# ── SNS topic for alarm delivery ─────────────────────────────────────────────
# Created only when alarm_email is set; alarms exist either way (visible in console).

resource "aws_sns_topic" "alarms" {
  count = var.alarm_email != "" ? 1 : 0
  name  = "${local.name_prefix}-alarms"

  tags = {
    Name = local.name_prefix
  }
}

resource "aws_sns_topic_subscription" "alarm_email" {
  count     = var.alarm_email != "" ? 1 : 0
  topic_arn = aws_sns_topic.alarms[0].arn
  protocol  = "email"
  endpoint  = var.alarm_email
}

locals {
  alarm_actions = var.alarm_email != "" ? [aws_sns_topic.alarms[0].arn] : []
}

# ── ECS: running task count drops below desired ───────────────────────────────

resource "aws_cloudwatch_metric_alarm" "ecs_running_tasks_low" {
  alarm_name          = "${local.name_prefix}-ecs-running-tasks-low"
  alarm_description   = "ECS running task count dropped below desired count — possible crash loop."
  comparison_operator = "LessThanThreshold"
  evaluation_periods  = 2
  metric_name         = "RunningTaskCount"
  namespace           = "ECS/ContainerInsights"
  period              = 60
  statistic           = "Average"
  threshold           = var.ecs_desired_count

  dimensions = {
    ClusterName = aws_ecs_cluster.main.name
    ServiceName = aws_ecs_service.app.name
  }

  alarm_actions             = local.alarm_actions
  ok_actions                = local.alarm_actions
  insufficient_data_actions = []

  treat_missing_data = "breaching"

  tags = {
    Name = local.name_prefix
  }
}

# ── ALB: HTTP 5xx spike ───────────────────────────────────────────────────────

resource "aws_cloudwatch_metric_alarm" "alb_5xx_high" {
  alarm_name          = "${local.name_prefix}-alb-5xx-high"
  alarm_description   = "ALB target 5xx errors exceeded 10 in a 5-minute window."
  comparison_operator = "GreaterThanThreshold"
  evaluation_periods  = 1
  metric_name         = "HTTPCode_Target_5XX_Count"
  namespace           = "AWS/ApplicationELB"
  period              = 300
  statistic           = "Sum"
  threshold           = 10

  dimensions = {
    LoadBalancer = aws_lb.main.arn_suffix
  }

  alarm_actions             = local.alarm_actions
  ok_actions                = local.alarm_actions
  insufficient_data_actions = []

  treat_missing_data = "notBreaching"

  tags = {
    Name = local.name_prefix
  }
}

# ── RDS: CPU utilisation high ─────────────────────────────────────────────────

resource "aws_cloudwatch_metric_alarm" "rds_cpu_high" {
  alarm_name          = "${local.name_prefix}-rds-cpu-high"
  alarm_description   = "RDS CPU utilisation above 80% for 10 minutes — check slow queries."
  comparison_operator = "GreaterThanThreshold"
  evaluation_periods  = 2
  metric_name         = "CPUUtilization"
  namespace           = "AWS/RDS"
  period              = 300
  statistic           = "Average"
  threshold           = 80

  dimensions = {
    DBInstanceIdentifier = aws_db_instance.main.identifier
  }

  alarm_actions             = local.alarm_actions
  ok_actions                = local.alarm_actions
  insufficient_data_actions = []

  treat_missing_data = "notBreaching"

  tags = {
    Name = local.name_prefix
  }
}

# ── RDS: free storage low ─────────────────────────────────────────────────────

resource "aws_cloudwatch_metric_alarm" "rds_free_storage_low" {
  alarm_name          = "${local.name_prefix}-rds-free-storage-low"
  alarm_description   = "RDS free storage below 2 GB — consider expanding allocated storage."
  comparison_operator = "LessThanThreshold"
  evaluation_periods  = 1
  metric_name         = "FreeStorageSpace"
  namespace           = "AWS/RDS"
  period              = 300
  statistic           = "Average"
  threshold           = 2147483648 # 2 GB in bytes

  dimensions = {
    DBInstanceIdentifier = aws_db_instance.main.identifier
  }

  alarm_actions             = local.alarm_actions
  ok_actions                = local.alarm_actions
  insufficient_data_actions = []

  treat_missing_data = "notBreaching"

  tags = {
    Name = local.name_prefix
  }
}
