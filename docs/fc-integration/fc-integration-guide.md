# Federated Catalogue (FC) Integration Guide

> **FC Commit Version:** [`deeae85d`](https://github.com/eclipse-xfsc/federated-catalogue/tree/deeae85dcffcf21583aed98754f6832d51bd9dcc)  
> **Document Maintainer:** Feifan Lin (<l.fin@neusta.de>)  

## Outline

- [TL;DR](#tldr)
- [Overview](#overview)
- [How We Use FC](#how-we-use-fc)
- [Core Data Model](#core-data-model)
  - [ContractTemplate](#contracttemplate)
  - [ServiceOffering](#serviceoffering)
  - [Relationships](#relationships)
- [Known Issues](#known-issues)
- [Development Environment](#development-environment)
  - [Docker Compose Configuration](#docker-compose-configuration)
- [Examples](#examples)
  - [Participant SD](#participant-sd)
  - [Service SD](#service-sd)
  - [Resource SD](#resource-sd)
  - [Schema](#schema)

## TL;DR

- FC is used for **template discovery**, not for business data storage
- SD types 
  - Define in (```fc-service-core/src/.../TrustFrameworkBaseClass.java```):
    - Participant
    - Resource
    - ServiceOffering
- Relationship model:
  - Template SD --(gax-core:operatedBy)--> Participant SD
  - ServiceOffering SD --(gax-core:offeredBy)--> Participant SD
  - ServiceOffering SD --(gax-trust-framework:endPointURL)--> DCS endpoint (string)
- Use `POST /self-description` create SD JSON-LD: 
  - participants
  - DCS services
  - template resources
- Use `POST /query` (OpenCypher) to retrieve the template SD and the corresponding DCS endpoint URL
- Use `GET /self-description` to enrich the template SD with additional metadata (e.g., sdHash, uploadDatetime)
- Disable `FEDERATED_CATALOGUE_VERIFICATION_VP_SIGNATURE` and `FEDERATED_CATALOGUE_VERIFICATION_VC_SIGNATURE` in the development environment to skip EV-SSL certificate validation and VC signature verification
- Known issue: Graph DB configuration is not properly initialized; as a result, `POST /query` does not work. (see the [Known Issues](#known-issues) section)

## Overview

The Federated Catalogue (FC) is a **decentralized system for storing and querying Self-Descriptions**.

Key responsibilities:

- Verifies Self-Descriptions
- Stores raw JSON-LD (resource / service / participant) in PostgreSQL
- Transforms JSON-LD into a graph structure (Neo4j)
- Provides advanced query capabilities via OpenCypher (`POST /query`)
- Synchronizes data across multiple FC instances

FC **does not handle business execution logic**.

References:

- ```SRS_GXFS_FC_CCF.pdf```, 
	-  page 2: 1.2 Product Scope
	-  page 10: Federation of Catalogues

## How We Use FC

Our use case focuses on:

- Submit template schema (ontology + SCHACL) to FC
- Every new DCS instance should Init participant and service SD to FC
- Publishing approval contract templates (resource SD) to FC
- Discovering templates on FC
- Resolving the corresponding DCS endpoint by template DID
- Start DCS-to-DCS negotiation

## Core Data Model

### ContractTemplate

Fields:

- `did`
- `documentNumber`
- `version`
- `participantId`
- ...

### ServiceOffering

Fields:

- `endPointURL`

### Relationships

ContractTemplate → operatedBy → Participant ← offeredBy ← ServiceOffering

## Known Issues

**Current git commit:** `deeae85dcffcf21583aed98754f6832d51bd9dcc`

[Commits link](https://github.com/eclipse-xfsc/federated-catalogue/commits/main/?after=0e7dcea977543d470c37e4d34f151c26b24f1950+34)

### Issue 1: GraphDbConfig not loaded

Reference fix: [Enhancement CAT-FR-GD-03 - graph database backend](https://github.com/eclipse-xfsc/federated-catalogue/commit/89ebfbfc0a34c37292bb1cd45339402fe5116c03#diff-a235525aedb1d24ed4dd19647ffcbd5e36d0898717e24538f2d33d1b61010c2dL15)

Impact:
  
- Graph DB configuration is not initialized.
- SDs are stored in PostgreSQL, but not written into Neo4j
- `POST /query` doesn't work

**Fix:**

Add missing package to component scan, ```fc-service-server/src/main/java.../ServiceConfig.java```:

```java
@ComponentScan(basePackages = {"eu.xfsc.fc.graphdb.service", "eu.xfsc.fc.graphdb.config"})
```

## Development Environment

### Docker Compose Configuration

In the development environment, configure the following variables under `services.server.environment` in `docker-compose.yml` before starting FC:

```yaml
services:
  server:
    environment:
      # Add the following at the end of the environment variables

      # Disable VP/VC signature verification in local development
      FEDERATED_CATALOGUE_VERIFICATION_VP_SIGNATURE: "false"
      FEDERATED_CATALOGUE_VERIFICATION_VC_SIGNATURE: "false"

      # Keycloak and JWT issuer configuration
      KEYCLOAK_AUTH_SERVER_URL: "http://keycloak:8080"
      SPRING_SECURITY_OAUTH2_RESOURCESERVER_JWT_ISSUER_URI: "http://keycloak:8080/realms/<your-realm>"

      # This FC instance (self)
      FEDERATED_CATALOGUE_QUERY_SELF: "http://host.docker.internal:8081"

      # Partner FC instances (index starts at 0)
      FEDERATED_CATALOGUE_QUERY_PARTNERS_0: "http://host.docker.internal:18081"
      FEDERATED_CATALOGUE_QUERY_PARTNERS_1: "http://host.docker.internal:28081"
      # Add more partners as needed:
      # FEDERATED_CATALOGUE_QUERY_PARTNERS_2: "http://host.docker.internal:38081"
      # FEDERATED_CATALOGUE_QUERY_PARTNERS_3: "http://host.docker.internal:48081"
```

In production, VP/VC verification may be temporarily disabled until EV-SSL certificates are available and signature verification is fully implemented.

## Examples

### Participant SD

Replace `gx-participant:*` fields with the actual participant information

FC Reference: [test-issuer.jsonld](https://github.com/eclipse-xfsc/federated-catalogue/blob/deeae85dcffcf21583aed98754f6832d51bd9dcc/examples/queries/test-issuer.jsonld)

Example: [Participant SD](./examples/participant_sd.jsonld)

### Service SD

FC Reference: [serviceOffering.jsonld](https://github.com/eclipse-xfsc/federated-catalogue/blob/deeae85dcffcf21583aed98754f6832d51bd9dcc/examples/serviceOffering.jsonld)

Example: [Service SD](./examples/service_sd.jsonld)

### Resource SD

FC Reference: [resource.jsonld](https://github.com/eclipse-xfsc/federated-catalogue/blob/deeae85dcffcf21583aed98754f6832d51bd9dcc/examples/resource.jsonld)

Example: [Resource SD](./examples/template_resource_sd.jsonld)

### Schema

Example: [Schema](./examples/template_schema.ttl)