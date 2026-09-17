output "aws_region" {
  description = "Deployed region."
  value       = var.aws_region
}

output "vpc_id" {
  description = "VPC ID."
  value       = aws_vpc.main.id
}

output "nat_public_ip" {
  description = "Static egress IP — whitelist at Zerodha (and Angel/Dhan when those adapters are enabled)."
  value       = aws_eip.nat.public_ip
}

output "ecr_repository_url" {
  description = "Docker push target."
  value       = aws_ecr_repository.app.repository_url
}

output "ecs_cluster_name" {
  description = "ECS cluster name."
  value       = aws_ecs_cluster.main.name
}

output "ecs_service_name" {
  description = "ECS service name."
  value       = aws_ecs_service.app.name
}

output "alb_dns_name" {
  description = "ALB DNS name (HTTP smoke when no custom domain)."
  value       = aws_lb.main.dns_name
}

output "alb_arn" {
  description = "ALB ARN (for HSTS attribute CLI)."
  value       = aws_lb.main.arn
}

output "api_url" {
  description = "Base URL for API smoke tests."
  value       = local.api_url
}

output "health_check_url" {
  description = "GET /healthz URL."
  value       = "${local.api_url}/healthz"
}

output "enabled_adapters" {
  description = "Execution adapters enabled in Core (ENABLED_ADAPTERS)."
  value       = sort(tolist(var.enabled_adapters))
}

output "execution_adapter_mode" {
  description = "Core EXECUTION_ADAPTER_MODE."
  value       = var.execution_adapter_mode
}

output "deployed_sidecar_adapters" {
  description = "Adapter sidecars provisioned as ECS services (subset of enabled_adapters)."
  value       = sort(tolist(local.ecs_sidecar_adapters))
}

output "zerodha_adapter_url" {
  description = "Public or internal Zerodha adapter base URL wired into Core ZERODHA_ADAPTER_URL."
  value       = local.zerodha_adapter_url
}

output "app_secret_arn" {
  description = "Secrets Manager ARN (DATABASE_URL, JWT_SECRET, AES_KEY)."
  value       = aws_secretsmanager_secret.app.arn
}

output "rds_endpoint" {
  description = "RDS hostname (migrations via DATABASE_URL secret)."
  value       = aws_db_instance.main.address
}

output "redis_endpoint" {
  description = "ElastiCache primary endpoint."
  value       = aws_elasticache_cluster.main.cache_nodes[0].address
}

output "ecr_dashboard_repository_url" {
  description = "Dashboard Docker push target."
  value       = aws_ecr_repository.dashboard.repository_url
}

output "app_url" {
  description = "Dashboard base URL."
  value       = local.app_url
}

output "next_steps" {
  description = "Post-apply checklist."
  value       = <<-EOT
    1. docker build && push to ${aws_ecr_repository.app.repository_url}:${var.image_tag}
    2. aws ecs update-service --cluster ${aws_ecs_cluster.main.name} --service ${aws_ecs_service.app.name} --force-new-deployment
    %{if length(local.ecs_sidecar_adapters) > 0~}
    3. Force-deploy adapter services: ${join(", ", [for k in sort(tolist(local.ecs_sidecar_adapters)) : "${local.name_prefix}-${k}-adapter"])}
    %{else~}
    3. (No adapter sidecars — execution_adapter_mode=${var.execution_adapter_mode})
    %{endif~}
    4. export DATABASE_URL from Secrets Manager; run deploy/scripts/migrate-rds.sh
    5. curl ${local.api_url}/healthz
  EOT
}
