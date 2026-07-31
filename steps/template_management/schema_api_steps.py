from behave import when
from steps.support.api_client import post_json, template_create_url

@when('I create a schema "{schema_name}"')
def step_when_create_schema(context, schema_name):
    payload = {
        "name": schema_name,
        "description": "BDD executable schema creation",

        # FIX: required by API at root level
        "template_type": "contract",

        "schema_data": {
            "type": "object",
            "properties": {
                "did": {"type": "string"},
                "version": {"type": "integer", "minimum": 0},
                "schema_version": {"type": "integer", "minimum": 1},
                "name": {"type": "string"},
                "created_at": {"type": "string", "format": "date-time"},
                "template_data": {
                    "type": "object",
                    "properties": {
                        "document_blocks": {
                            "type": "array",
                            "items": {
                                "type": "object",
                                "properties": {
                                    "type": {
                                        "type": "string",
                                        "enum": ["SECTION", "TEXT", "CLAUSE"]
                                    },
                                    "block_id": {"type": "string"},
                                    "text": {"type": "string"},
                                    "title": {"type": "string"},
                                    "condition_ids": {
                                        "type": "array",
                                        "items": {"type": "string"}
                                    }
                                },
                                "required": ["type", "block_id", "text"]
                            }
                        }
                    },
                    "required": ["document_blocks"]
                }
            },
            "required": [
                "version",
                "schema_version",
                "name",
                "created_at",
                "template_data"
            ]
        }
    }

    response = post_json(context, template_create_url(context), payload)
    context.requests_response = response

    assert response.status_code in (200, 201), response.text

    body = response.json()
    context.created_schema_did = body.get("did")

    assert context.created_schema_did, body