resource "aws_mwaa_environment" "this" {
  name              = var.name
  airflow_version   = var.airflow_version
  environment_class = var.environment_class

  execution_role_arn = aws_iam_role.mwaa.arn
  source_bucket_arn  = aws_s3_bucket.mwaa.arn

  dag_s3_path          = "dags/"
  requirements_s3_path = "requirements.txt"

  # Public UI so we can open the Airflow webserver without a bastion.
  webserver_access_mode = "PUBLIC_ONLY"

  # mw1.micro runs a single scheduler/worker container and a single webserver.
  schedulers     = 1
  min_workers    = 1
  max_workers    = 1
  min_webservers = 1
  max_webservers = 1

  network_configuration {
    security_group_ids = [aws_security_group.mwaa.id]
    subnet_ids         = module.vpc.private_subnets
  }

  logging_configuration {
    dag_processing_logs {
      enabled   = true
      log_level = "INFO"
    }
    scheduler_logs {
      enabled   = true
      log_level = "INFO"
    }
    task_logs {
      enabled   = true
      log_level = "INFO"
    }
    webserver_logs {
      enabled   = true
      log_level = "INFO"
    }
    worker_logs {
      enabled   = true
      log_level = "INFO"
    }
  }

  depends_on = [
    aws_iam_role_policy.mwaa,
    aws_s3_object.requirements,
    aws_s3_object.dags_keep,
    aws_s3_bucket_versioning.mwaa,
  ]
}
