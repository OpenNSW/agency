# NSW Integration

This document explains how the Agency service integrates with the NSW Core Workflow Engine (CWE) and the broader NSW platform.

## Overview

The Agency service participates in the NSW consignment workflow as a verification step. When a trader submits a form that requires agency verification, the NSW workflow engine:

1. Injects the submission data into the appropriate Agency instance
2. Waits for a callback indicating the review outcome
3. Advances the workflow based on the decision

```
Trader                  NSW CWE                Agency Service           Officer
  │                       │                        │                      │
  │── Submit form ───────▶│                        │                      │
  │                       │── POST /inject ───────▶│                      │
  │                       │◀── 201 Created ────────│                      │
  │                       │                        │                      │
  │                       │   (task waits...)       │                      │
  │                       │                        │◀── GET applications ──│
  │                       │                        │── Return with form ──▶│
  │                       │                        │                      │
  │                       │                        │◀── POST review ──────│
  │                       │◀── POST callback ──────│                      │
  │                       │                        │                      │
  │◀── Status updated ────│                        │                      │
```

## SimpleForm Plugin

On the NSW side, the Agency integration is handled by the **SimpleForm** plugin (`backend/internal/task/plugin/simple_form.go`). This plugin manages the full lifecycle:

### Plugin States

```
Initialized ──▶ TraderSavedAsDraft ──▶ TraderSubmitted ──▶ AgencyAcknowledged ──▶ AgencyReviewed
```

- **Initialized** -- Form loaded, waiting for trader input
- **TraderSavedAsDraft** -- Trader saved a draft (optional)
- **TraderSubmitted** -- Trader submitted the form; if `requiresAgencyVerification` is false, the task completes here
- **AgencyAcknowledged** -- Data injected into NSW Agency, waiting for review callback
- **AgencyReviewed** -- Agency callback received, task completed or failed based on decision

### Workflow Node Configuration

Each Agency verification task in the workflow is configured with submission and callback settings:

```json
{
  "agency": "NPQS",
  "formId": "22222222-2222-2222-2222-222222222222",
  "service": "plant-quarantine-phytosanitary",
  "requiresAgencyVerification": true,
  "submission": {
    "url": "http://localhost:8081/api/v1/inject",
    "request": {
      "meta": {
        "type": "consignment",
        "verificationId": "moa:npqs:phytosanitary:001"
      }
    }
  },
  "callback": {
    "response": {
      "display": {
        "formId": "d0c3b860-635b-4124-8081-d3f421e429cb"
      },
      "mapping": {
        "reviewedAt": "gi:phytosanitary:meta:reviewedAt",
        "reviewerNotes": "gi:phytosanitary:meta:reviewNotes"
      }
    }
  }
}
```

Key fields:
- **`submission.url`** -- The inject endpoint for this agency
- **`submission.request.meta`** -- Metadata that determines which review form the Agency officer sees
- **`callback.response.display.formId`** -- Form used to display the Agency response back in the trader portal
- **`callback.response.mapping`** -- Maps callback fields into the workflow's global context

## Callback Contract

When an Agency officer reviews an application, the Agency service POSTs a callback to the NSW task endpoint it derives from its own `NSW_API_BASE_URL` -- `{NSW_API_BASE_URL}/api/v1/tasks/{taskId}` (typically `http://localhost:8080/api/v1/tasks/{taskId}`):

```json
{
  "task_id": "927adaaa-b959-4648-880a-16508acafc12",
  "consignment_id": "cefda05e-3071-4e94-b001-328094e570a7",
  "payload": {
    "action": "AGENCY_VERIFICATION",
    "content": {
      "decision": "APPROVED",
      "phytosanitaryClearance": "CLEARED",
      "inspectionReference": "NPQS/2024/001",
      "remarks": "OK"
    }
  }
}
```

The NSW backend processes this callback:

1. Looks up the task by `task_id`
2. Validates that `consignment_id` matches
3. Passes the payload to `plugin.Execute()` with action `AGENCY_VERIFICATION`
4. The SimpleForm plugin stores the Agency response in its local state
5. Based on the `decision` field:
   - `"APPROVED"` -- task state set to `Completed`
   - Anything else -- task state set to `Failed`
6. Mapped fields are written to the workflow's global context

## End-to-End Example: Desiccated Coconut Export

A typical workflow for exporting desiccated coconut includes these tasks:

1. **General Information** -- Trader enters consignee details
2. **Customs Declaration** -- Export declaration form
3. **Phytosanitary Certificate** -- NPQS verification (NSW Agency, port 8081)
4. **Health Certificate** -- FCAU verification (NSW Agency, port 8082)
5. **Final Processing** -- Wait for completion

For tasks 3 and 4:
- Trader fills out the agency-specific form in the NSW portal
- On submission, the SimpleForm plugin POSTs to the respective Agency instance
- The agency officer reviews in their agency portal and submits a decision
- The Agency service sends the callback, and the CWE advances to the next task

## Currently Supported Agencies

| Agency | Service | Port | Verification ID |
|---|---|---|---|
| NPQS (National Plant Quarantine Service) | Phytosanitary certification | 8081 | `moa:npqs:phytosanitary:001` |
| FCAU (Food Control Administration Unit) | Health certificate | 8082 | `moh:fcau:health_cert:001` |

## Adding a New Agency

To integrate a new NSW Agency:

1. **Start a new Agency instance** on a dedicated port with its own database:
   ```bash
   PORT=8083 DB_PATH=./new_agency.db go run ./cmd/server
   ```

2. **Create a review form** in `data/forms/` (see [Dynamic Forms](dynamic-forms.md))

3. **Add workflow node configuration** in the NSW backend migrations with:
   - `submission.url` pointing to the new Agency instance
   - `submission.request.meta` matching the new form's ID
   - `callback.response.mapping` for any fields that need to flow into the workflow context

4. **Add seed data** for the trader-facing form in the NSW backend's `forms` table

No changes to agency application code are needed -- adding a new agency is purely a configuration and data concern.
