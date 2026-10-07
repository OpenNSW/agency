# API Reference

All endpoints accept and return JSON. Errors are returned as `{"error": "message"}`.

## Health Check

```
GET /health
```

**Response** `200 OK`

```json
{
  "status": "ok",
  "service": "agency-portal"
}
```

## Inject Data

Called by the NSW workflow engine to submit data for Agency review.

```
POST /api/v1/inject
```

**Request Body**

| Field | Type | Required | Description |
|---|---|---|---|
| `taskId` | string | Yes | Task identifier from the workflow |
| `callbackToken` | string | Yes | Opaque token naming the NSW workflow step this inject is for. Stored and used to send the review outcome back (see Callback Payload). A re-inject of the same task carries a new one. |
| `consignmentId` | string | Yes | Parent consignment identifier |
| `data` | object | No | Trader-submitted data to display during review |
| `meta` | object | No | Metadata for form selection (see [Dynamic Forms](dynamic-forms.md)) |
| `meta.type` | string | -- | Verification type (e.g., `"consignment"`) |
| `meta.verificationId` | string | -- | Verification identifier (e.g., `"moa:npqs:phytosanitary:001"`) |

**Example Request**

```bash
curl -X POST http://localhost:8081/api/v1/inject \
  -H "Content-Type: application/json" \
  -d '{
    "taskId": "927adaaa-b959-4648-880a-16508acafc12",
    "callbackToken": "1mq-vSNmKRc6aKXmHSpHvBDTBvHKEn0BBl8wJ_E6WfOI",
    "consignmentId": "cefda05e-3071-4e94-b001-328094e570a7",
    "data": {
      "countryOfOrigin": "LK",
      "countryOfDestination": "UK",
      "distinguishingMarks": "BWI-UK-LOT01"
    },
    "meta": {
      "type": "consignment",
      "verificationId": "moa:npqs:phytosanitary:001"
    }
  }'
```

**Response** `201 Created`

```json
{
  "success": true,
  "message": "Data injected successfully",
  "taskId": "927adaaa-b959-4648-880a-16508acafc12"
}
```

**Error Responses**

| Status | Condition |
|---|---|
| `400` | Missing required fields or invalid JSON |
| `500` | Database error |

## List Applications

Returns a paginated list of applications for the agency officer portal.

```
GET /api/v1/applications
```

**Query Parameters**

| Parameter | Type | Default | Description |
|---|---|---|---|
| `status` | string | _(all)_ | Filter by status: `PENDING`, `APPROVED`, `REJECTED` |
| `page` | int | `1` | Page number (1-indexed) |
| `pageSize` | int | `20` | Items per page (max 100) |

**Example Request**

```bash
curl "http://localhost:8081/api/v1/applications?status=PENDING&page=1&pageSize=10"
```

**Response** `200 OK`

```json
{
  "items": [
    {
      "taskId": "927adaaa-b959-4648-880a-16508acafc12",
      "consignmentId": "cefda05e-3071-4e94-b001-328094e570a7",
        "data": {
        "countryOfOrigin": "LK",
        "countryOfDestination": "UK"
      },
      "meta": {
        "type": "consignment",
        "verificationId": "moa:npqs:phytosanitary:001"
      },
      "status": "PENDING",
      "createdAt": "2024-01-27T10:00:00Z",
      "updatedAt": "2024-01-27T10:00:00Z"
    }
  ],
  "total": 45,
  "page": 1,
  "pageSize": 10
}
```

## Get Application

Returns a single application with the appropriate review form attached.

```
GET /api/v1/applications/{taskId}
```

**Path Parameters**

| Parameter | Type | Description |
|---|---|---|
| `taskId` | string | The application's task identifier |

**Example Request**

```bash
curl "http://localhost:8081/api/v1/applications/927adaaa-b959-4648-880a-16508acafc12"
```

**Response** `200 OK`

```json
{
  "taskId": "927adaaa-b959-4648-880a-16508acafc12",
  "consignmentId": "cefda05e-3071-4e94-b001-328094e570a7",
  "data": {
    "countryOfOrigin": "LK",
    "countryOfDestination": "UK"
  },
  "meta": {
    "type": "consignment",
    "verificationId": "moa:npqs:phytosanitary:001"
  },
  "form": {
    "schema": {
      "type": "object",
      "required": ["decision", "phytosanitaryClearance"],
      "properties": {
        "decision": { "type": "string", "title": "Decision", "oneOf": [{"const": "APPROVED", "title": "Approved"}, {"const": "REJECTED", "title": "Rejected"}] },
        "phytosanitaryClearance": { "type": "string", "title": "Phytosanitary Clearance Status" },
        "inspectionReference": { "type": "string", "title": "Inspection / Certificate Reference No" },
        "remarks": { "type": "string", "title": "NPQS Remarks" }
      }
    },
    "uiSchema": { "type": "VerticalLayout", "elements": ["..."] }
  },
  "status": "PENDING",
  "createdAt": "2024-01-27T10:00:00Z",
  "updatedAt": "2024-01-27T10:00:00Z"
}
```

The `form` field contains a [JSON Forms](https://jsonforms.io/) definition that the frontend uses to render the review UI. The form is selected based on the application's `meta` field (see [Dynamic Forms](dynamic-forms.md)).

**Error Responses**

| Status | Condition |
|---|---|
| `400` | Invalid or missing `taskId` |
| `404` | Application not found |

## Review Application

Submit a review decision. This updates the application status and POSTs a callback to the originating service.

```
POST /api/v1/applications/{taskId}/review
```

**Path Parameters**

| Parameter | Type | Description |
|---|---|---|
| `taskId` | string | The application's task identifier |

**Request Body**

The body is dynamic based on the review form, but must always include a `decision` field:

| Field | Type | Required | Description |
|---|---|---|---|
| `decision` | string | Yes | `"APPROVED"` or `"REJECTED"` |
| _(other fields)_ | any | Varies | Additional fields defined by the review form |

**Example Request (default form)**

```bash
curl -X POST http://localhost:8081/api/v1/applications/927adaaa-b959-4648-880a-16508acafc12/review \
  -H "Content-Type: application/json" \
  -d '{
    "decision": "APPROVED",
    "remarks": "All documents verified successfully"
  }'
```

**Example Request (NPQS phytosanitary form)**

```bash
curl -X POST http://localhost:8081/api/v1/applications/927adaaa-b959-4648-880a-16508acafc12/review \
  -H "Content-Type: application/json" \
  -d '{
    "decision": "APPROVED",
    "phytosanitaryClearance": "CLEARED",
    "inspectionReference": "NPQS/2024/001",
    "remarks": "Fumigation records acceptable"
  }'
```

**Response** `200 OK`

```json
{
  "success": true,
  "message": "Application reviewed successfully"
}
```

**Callback Payload**

After a successful review, the service POSTs the following to the NSW callback
endpoint, addressed by the application's callback token
(`{NSW_API_BASE_URL}/api/v1/callbacks/{callbackToken}`):

```json
{
  "command": "approve",
  "payload": {
    "decision": "APPROVED",
    "phytosanitaryClearance": "CLEARED",
    "inspectionReference": "NPQS/2024/001",
    "remarks": "Fumigation records acceptable"
  }
}
```

`command` is the outcome read from the reviewer response's outcome field
(`behavior.outcomeField` in the task config), or `approve` when there is none, and
`payload` is the reviewer response. NSW answers `409` if the step the
token names is no longer active (the task has moved on), and the review fails.

The `content` field contains the entire review body as submitted by the officer.

**Error Responses**

| Status | Condition |
|---|---|
| `400` | Missing `decision` field or invalid JSON |
| `404` | Application not found |
| `500` | Database error or callback delivery failure |