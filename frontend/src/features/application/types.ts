import type { JsonSchema, UISchemaElement } from '@jsonforms/core'

export interface SchemaOption {
  const: unknown
  title?: string
}

export interface SchemaProperty {
  oneOf?: SchemaOption[]
  enum?: string[]
}

export interface FeedbackEntry {
  content: Record<string, unknown>
  timestamp: string
  round: number
}

export interface FormDefinition {
  schema: JsonSchema
  uiSchema: UISchemaElement
}

export interface AgencyApplication {
  taskId: string
  consignmentId: string

  // Trader-submitted data and reviewer action data — only present on the
  // detail response (GET /applications/{taskId}), omitted from list results.
  data?: Record<string, unknown>
  agencyActionData?: Record<string, unknown>

  // Task metadata from config
  title?: string
  description?: string
  icon?: string
  category?: string

  // Form definitions
  dataForm?: FormDefinition
  agencyForm?: FormDefinition

  // RBAC — only present on the detail response
  allowedActions?: string[]

  // Set when this task's officer can generate a certificate (see
  // generateCertificate in service.ts) — only present on the detail response
  certificateTemplateId?: string
  // JSON Schema for what's required before generating the certificate —
  // deliberately separate from the review form's own schema, since some of
  // its required fields (a signed certificate upload, an authorized
  // signature) can only be provided after the certificate has been
  // generated and printed.
  certificateDataSchema?: JsonSchema

  status: string
  feedbackHistory?: FeedbackEntry[]
  reviewerNotes?: string
  reviewedAt?: string
  createdAt: string
  updatedAt: string
}

export interface UploadMetadataRequest {
  filename: string
  mime_type: string
  size: number
}

export interface UploadMetadataResponse {
  key: string
  name: string
  upload_url: string
}

export interface DownloadMetadataResponse {
  download_url: string
  expires_at: number
}

export interface UploadResponse {
  key: string
  name: string
}
