resource "aws_db_parameter_group" "main" {
  name        = "${local.name_prefix}-pg16"
  family      = "postgres16"
  description = "ZettaBridge staging - enables pg_cron for scheduled data retention"

  parameter {
    name         = "shared_preload_libraries"
    value        = "pg_stat_statements,pg_tle,pg_cron"
    apply_method = "pending-reboot"
  }

  tags = {
    Name = local.name_prefix
  }
}

resource "aws_db_subnet_group" "main" {
  name       = local.name_prefix
  subnet_ids = aws_subnet.private[*].id

  tags = {
    Name = local.name_prefix
  }
}

resource "aws_db_instance" "main" {
  identifier = local.name_prefix

  engine         = "postgres"
  engine_version = "16"
  instance_class = var.db_instance_class

  allocated_storage     = var.db_allocated_storage_gb
  storage_type          = "gp3"
  db_name               = local.db_name
  username              = local.db_username
  password              = random_password.db_password.result
  port                  = 5432

  db_subnet_group_name   = aws_db_subnet_group.main.name
  parameter_group_name   = aws_db_parameter_group.main.name
  vpc_security_group_ids = [aws_security_group.rds.id]

  publicly_accessible = false
  multi_az            = false

  backup_retention_period = 7
  skip_final_snapshot     = true
  deletion_protection     = false

  storage_encrypted = true

  tags = {
    Name = local.name_prefix
  }
}
