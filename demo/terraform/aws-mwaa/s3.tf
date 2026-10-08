resource "aws_s3_bucket" "mwaa" {
  bucket_prefix = "orders-demo-mwaa-"
  force_destroy = true
}

# MWAA requires versioning on the source bucket.
resource "aws_s3_bucket_versioning" "mwaa" {
  bucket = aws_s3_bucket.mwaa.id

  versioning_configuration {
    status = "Enabled"
  }
}

resource "aws_s3_bucket_public_access_block" "mwaa" {
  bucket = aws_s3_bucket.mwaa.id

  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}

# Placeholder empty requirements file so the environment has a valid
# requirements_s3_path to start from.
resource "aws_s3_object" "requirements" {
  bucket       = aws_s3_bucket.mwaa.id
  key          = "requirements.txt"
  content      = ""
  content_type = "text/plain"
}

# Keep the dags/ prefix present in the bucket.
resource "aws_s3_object" "dags_keep" {
  bucket       = aws_s3_bucket.mwaa.id
  key          = "dags/.keep"
  content      = ""
  content_type = "text/plain"
}
