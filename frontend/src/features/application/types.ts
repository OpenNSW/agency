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
  serviceUrl: string
  data: Record<string, unknown>
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
