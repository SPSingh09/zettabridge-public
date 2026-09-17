terraform {
  required_version = ">= 1.5.0"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
    random = {
      source  = "hashicorp/random"
      version = "~> 3.6"
    }
  }

  # Remote state — uncomment AFTER creating the S3 bucket and DynamoDB table:
  #   aws s3api create-bucket --bucket zettabridge-terraform-state --region ap-south-1 --create-bucket-configuration LocationConstraint=ap-south-1
  #   aws s3api put-bucket-versioning --bucket zettabridge-terraform-state --versioning-configuration Status=Enabled
  #   aws dynamodb create-table --table-name zettabridge-terraform-locks --attribute-definitions AttributeName=LockID,AttributeType=S --key-schema AttributeName=LockID,KeyType=HASH --billing-mode PAY_PER_REQUEST --region ap-south-1
  #
  backend "s3" {
    bucket         = "zettabridge-terraform-state"
    key            = "staging/terraform.tfstate"
    region         = "ap-south-1"
    encrypt        = true
    dynamodb_table = "zettabridge-terraform-locks"
  }
}
