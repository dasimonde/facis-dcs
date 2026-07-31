# FACIS DCS vocabulary namespace — identity and resolution

The vocabulary namespace (`dcs:`/`dcst:` term IRIs) is the *identity* of
the FACIS vocabulary, shared by every document in the ecosystem. Two
instances minting terms under different namespaces produce RDF that no
longer unifies — `dcs:title` from one is a different predicate than
`dcs:title` from the other, shapes stop targeting each other's data, and
the ecosystem decays into dialect islands. The namespace is therefore a
**global constant, not a per-instance setting**.

Until the w3id registration below is filed, the w3id IRIs are stable
identifiers that do not yet resolve on the open web — and that costs the
system nothing at runtime:

- JSON-LD processing only ever dereferences `@context` URLs, never term
  IRIs. Produced documents anchor `@context` to the hub's own versioned
  URL, and the in-process document loader additionally serves the hub
  copy for the historical w3id context IRI — so expansion and validation
  never depend on w3id resolving.
- The vocabulary documents themselves stay fetchable from any instance's
  public `/semantic/ontology/facis-sla`, `/semantic/context/facis-dcs`,
  and `/semantic/shapes/facis-dcs` routes.

What the registration adds is third-party follow-your-nose: an external
agent pasting a bare `dcs:` term IRI into a resolver reaches the shapes
that constrain it.

# w3id.org registration for the FACIS DCS namespaces

The `https://w3id.org/facis/...` IRIs used throughout FACIS DCS documents
(ontology terms, taxonomy values, the historical context/shapes
identifiers) resolve through [w3id.org](https://w3id.org), a community-run
permanent-identifier redirect service. Registration is a pull request to
<https://github.com/perma-id/w3id.org> adding the `facis/` directory with
the `.htaccess` in this folder.

Before filing the PR, replace `FACIS_PUBLIC_HOST` in `.htaccess` with the
canonical public DCS deployment origin (scheme + host + API base path,
e.g. `https://dcs.example.org/digital-contracting-service/api`). The
redirect targets are the Semantic Hub's public resolution routes:

| w3id IRI                                   | resolves to                          |
|--------------------------------------------|--------------------------------------|
| `/facis/dcs/ontology/v1` (and `#term`)     | `/semantic/shapes/facis-dcs`         |
| `/facis/dcs/context/v1`                    | `/semantic/context/facis-dcs`        |
| `/facis/dcs/shapes/v1`                     | `/semantic/shapes/facis-dcs`         |
| `/facis/dcs/taxonomy/v1` (and `#term`)     | `/semantic/ontology/facis-sla`       |

There is no envelope-ontology document behind `dcs:`. The terms are
constrained by the shapes graph and by nothing else, so their IRIs
dereference to the shapes rather than to a vocabulary document that would
describe them without constraining them. `dcst:` taxonomy values are
fragments on the domain-field ontology, which is where they are declared.

All four routes are public and unauthenticated (`backend/design/
semantic_hub.go`), so the redirect targets dereference without a DCS
login. Fragment identifiers (`#ContractMetadata`, `#role-provider`, ...)
are preserved by the redirect per RFC 7231 and land on the served
ontology document.
