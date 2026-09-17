resource "aws_acm_certificate" "api" {
  count = var.enable_https && var.acm_certificate_arn == "" && var.api_domain_name != "" ? 1 : 0

  domain_name       = var.api_domain_name
  validation_method = "DNS"

  lifecycle {
    create_before_destroy = true
  }

  tags = {
    Name = local.name_prefix
  }
}

resource "aws_route53_record" "cert_validation" {
  for_each = var.enable_https && var.acm_certificate_arn == "" && var.api_domain_name != "" && var.route53_zone_id != "" ? {
    for dvo in aws_acm_certificate.api[0].domain_validation_options : dvo.domain_name => {
      name   = dvo.resource_record_name
      record = dvo.resource_record_value
      type   = dvo.resource_record_type
    }
  } : {}

  zone_id = var.route53_zone_id
  name    = each.value.name
  type    = each.value.type
  ttl     = 60
  records = [each.value.record]
}

resource "aws_acm_certificate_validation" "api" {
  count = length(aws_route53_record.cert_validation) > 0 ? 1 : 0

  certificate_arn         = aws_acm_certificate.api[0].arn
  validation_record_fqdns = [for r in aws_route53_record.cert_validation : r.fqdn]
}

locals {
  tls_certificate_arn = var.enable_https ? (
    var.acm_certificate_arn != "" ? var.acm_certificate_arn : (
      length(aws_acm_certificate_validation.api) > 0 ? aws_acm_certificate_validation.api[0].certificate_arn : try(aws_acm_certificate.api[0].arn, "")
    )
  ) : ""
}

resource "aws_route53_record" "api" {
  count = var.enable_https && var.api_domain_name != "" && var.route53_zone_id != "" ? 1 : 0

  zone_id = var.route53_zone_id
  name    = var.api_domain_name
  type    = "A"

  alias {
    name                   = aws_lb.main.dns_name
    zone_id                = aws_lb.main.zone_id
    evaluate_target_health = true
  }
}
