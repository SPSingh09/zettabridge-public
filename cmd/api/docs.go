package main

// @title           ZettaBridge API
// @version         1.0
// @description     REST and WebSocket API for ZettaBridge — TradingView webhook ingestion, paper and live broker execution, billing, and platform admin.
// @contact.name    ZettaBridge
// @contact.url     https://zettabridge.net
// @license.name    Proprietary
// @host            api.staging.zettabridge.net
// @BasePath        /
// @schemes         https http

// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
// @description JWT access token from POST /v1/auth/login. Prefix the value with "Bearer ".

// @securityDefinitions.apikey ServiceToken
// @in header
// @name Authorization
// @description Adapter→Core internal API token (Bearer value of ZB_SERVICE_TOKEN).

// @tag.name monitoring
// @tag.description Health checks and Prometheus metrics

// @tag.name auth
// @tag.description Registration, login, email verification, logout

// @tag.name account
// @tag.description Current user profile and plan limits

// @tag.name signals
// @tag.description TradingView webhook signal ingestion

// @tag.name streaming
// @tag.description Real-time trade WebSocket stream

// @tag.name webhooks
// @tag.description Webhook CRUD, ingest tokens, and trade audit

// @tag.name trades
// @tag.description Trade detail, export, and cancellation

// @tag.name credentials
// @tag.description Live broker credentials and OAuth

// @tag.name paper
// @tag.description Paper trading accounts, positions, and orders

// @tag.name billing
// @tag.description Subscription plans and Stripe checkout

// @tag.name internal
// @tag.description Adapter→Core callbacks (service token)

// @tag.name admin
// @tag.description Platform admin (requires role=admin JWT)

// @tag.name notifications
// @tag.description In-app notifications and Telegram alert settings

// @tag.name symbols
// @tag.description User symbol requests

// swaggerMeta is never called; it exists only so swag can attach package-level metadata.
func swaggerMeta() {}
