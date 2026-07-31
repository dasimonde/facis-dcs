# Architekturentscheidungen

Dieses Dokument sammelt Entscheidungen, die in der Software als Lösung
umgesetzt sind — insbesondere die vielen kleinen, nie formal als ADR
beschlossenen. Es ergänzt die ADRs (`docs/adr-*.md`), ersetzt sie nicht:
wo ein ADR existiert, wird verwiesen statt dupliziert. Gliederung nach
Systembereichen, nicht nach Fundort im Code.

Stand dieser Fassung: Backend-Kern (`backend/internal/base/`, `backend/cmd/`,
`backend/migrations/`), Backend-Fachschichten (`backend/internal/` übrige
Pakete, `backend/design/`), Rendering-Dienst (`pdf-core/`) und Web-Client
(`frontend/ClientApp/src/`).

## Vorlagen und Inhalte (Semantik, Validierung)

- **Was gilt:** Dokumente (Templates, Verträge) tragen als Anker die
  versionierten Hub-URLs für JSON-LD-Kontext (`@context`) und SHACL-Shapes
  (`sh:shapesGraph`); die Anker werden einmalig bei Produktion gesetzt, ein
  Dokument behält die Hub-Versionen, unter denen es verfasst wurde.
  **Warum:** Revalidierung muss gegen exakt die damals gültige Schema-Version
  laufen (ADR-8). Kein Validierungsprofil-Anker im Dokument: der Profilname
  sagte nichts über das Vokabular der eigentlichen Vertragsdaten aus
  (ADR-23); die Profilregeln laufen trotzdem zur Validierungszeit.
- **Was gilt:** Ein Dokument, dessen `@context` einen vom Semantic Hub
  deklarierten Prefix auf eine andere IRI umdefiniert, wird abgewiesen
  (`ErrDocumentSchemaConflict`, HTTP 400). Ebenso werden externe Kontext-IRIs
  abgewiesen, die nicht im Hub registriert sind. **Warum:** Validierung löst
  Kontexte hermetisch (ohne Netzzugriff) auf; eine unregistrierte IRI würde
  jedes spätere Audit des Dokuments scheitern lassen (DCS-FR-TR-03).
- **Was gilt:** Die Vertragshierarchie ist strikt child→parent: höchstens ein
  `dcs:parentContract`, niemals kindaufzählende Properties
  (`dcs:childContracts`, `dcs:subContracts`, `dcs:hasPart`) im Dokument.
  **Warum:** Eine Kindliste würde Geschwister an jeden Empfänger des
  Parent-Dokuments leaken und bei jedem neuen Kind ein Dokument-Rewrite
  erzwingen (ADR-7).
- **Was gilt:** Der Contract-Data-Graph ist selbst-enthalten: jedes
  Domänenobjekt ist ein typisierter, per `@id` adressierbarer Knoten; Werte
  sind Literale, Referenzen auf deklarierte `ContractField`s oder auf andere
  Domänenobjekte im Dokument; eingebettete Blank Nodes sind unzulässig.
  Absolute IRIs auf externe Ressourcen (sh:IRI-Blätter) sind erlaubt.
  **Warum:** SHACL-Traversierung und Rendering dürfen nichts außerhalb des
  Dokuments konsultieren (ADR-23).
- **Was gilt:** `dcs:policies` akzeptiert nur ein einzelnes umschließendes
  `odrl:Offer`/`odrl:Agreement` (Template: Offer; Vertrag: Offer, nach
  Signatur Agreement) mit `odrl:profile`, `@id` und vollständigen Regeln
  (genau eine `odrl:action` plus assigner/assignee/target, jede Regel mit
  `dcs:prose`-Rückbindung an die menschenlesbare Klausel). Nackte
  Regel-Arrays werden abgewiesen. **Warum:** Nur die kanonische Form ist von
  einem Standard-ODRL-Prozessor konsumierbar (ADR-6).
- **Was gilt:** ODRL-Constraint-Semantik ist einmal als Rego formuliert und
  läuft auf der eingebetteten OPA-Engine; ein Paritätstest sichert
  Verdikt-Gleichheit mit der historischen Go-Implementierung. Siehe ADR-11.
- **Was gilt:** SHACL-Validierung läuft auf goRDFlib (ADR-9); geparste
  Shapes-Graphen werden prozessweit nach Content-Hash gecacht (max. 8
  Einträge, dann Reset). **Warum:** Eine registrierte Shapes-Library kann
  Megabytes Turtle sein; Re-Parsen pro Request dominierte die Latenz. Der
  Graph wird nur gelesen, Lazy-Indizes werden beim Cache-Fill gewärmt —
  daher über nebenläufige Validierungen teilbar.
- **Was gilt:** Peer-seitige Validierung eines empfangenen Dokuments läuft
  gegen den Semantic Hub des Ursprungs (`RemoteShapeSource` mit expliziter
  ShapeSource statt des prozessweiten Zustands). **Warum:** Eine einmalige
  Remote-Hub-Validierung darf unter nebenläufigen Requests keinen geteilten
  Prozesszustand mutieren (ADR-23, Phase 4).
- **Was gilt:** Die FC-Schemas (`migrations/fcschemas`) werden beim Start
  inhaltsbasiert mit dem Federated Catalogue abgeglichen (Matching über
  Prefix/IRI-Vorkommen, nie Löschen im FC; Create-Konflikt wird per Update
  aufgelöst). **Warum:** FC vergibt eigene IDs; Inhalt ist die einzige
  stabile Identität.

- **Was gilt:** Der Semantic Hub versioniert unveränderlich: Eine registrierte
  Version wird nie editiert; das Seed beim Start registriert bei Inhalts-Drift
  der eingebetteten Assets eine neue aktive Version, ältere Versionen bleiben
  auflösbar. **Warum:** Dokumente pinnen Hub-Versionen (ADR-8); ein
  Asset-Update im Deployment darf gepinnte Anker nicht umdeuten.
- **Was gilt:** Hub-Reads (`/semantic/...`) sind öffentlich wie
  `/.well-known/did.json`. **Warum:** Produzierte Artefakte tragen
  Hub-Anker, die externe Verifier ohne DCS-Login dereferenzieren müssen.
- **Was gilt:** Per URL registrierte Schemata (Gaia-X u. ä. hinter
  w3id/purl-Redirects) werden als Byte-Snapshot eingefroren, nie als
  Live-Referenz gespeichert; der Fetch ist RBAC-gebunden und in Schema,
  Größe und Zeit begrenzt. Eine Kontext-Version muss mindestens als
  JSON-LD-Dokument mit `@context` parsen, bevor sie aktiviert wird.
- **Was gilt:** Der Clause-Katalog ist ein zweiter, unabhängig versionierter
  `kind="shapes"`-Eintrag (ADR-10); Template-Builder-Palette
  (`GET /semantic/clauses`) und Vertragsvalidierung lesen denselben
  Shapes-Graphen.
- **Was gilt:** Die Federated-Catalogue-Readiness wird mit exakt einem
  funktionalen Request geprüft; als Prüf-Payload dient die eingebettete
  Known-Good-Credential aus der Upstream-FC-Testsuite. Der native
  Actuator-Check läuft nur für co-deployte Kataloge — Remote-Betreiber
  exponieren Actuator-Endpoints üblicherweise nicht.

## Lebenszyklus (Workflow, Deployment, Betriebskonfiguration)

- **Was gilt:** `contractstate.Transitions` ist die einzige Quelle der
  Wahrheit für erlaubte (Zustand, Event)-Paare und deren Zielzustände; jede
  Command-Handler-Aktion validiert dagegen, die bestehende
  Task-Orchestrierung wählt nur unter den erlaubten Ausgängen.
  Verstöße wrappen `ErrInvalidTransition` und werden am HTTP-Rand als 400
  (nicht 500) gemappt; No-op-Übergänge (from == to) sind immer erlaubt.
- **Was gilt:** Einzelne Kanten sind bewusst so geschnitten: Withdraw ist ab
  APPROVED nicht mehr möglich; SIGNED→SIGNED deckt weitere Signatare eines
  Multi-Signer-Vertrags; ACTIVE→ACTIVE macht Deploy idempotent (die
  Signatur-Vollendung deployt automatisch, ein manueller Re-Dispatch und
  dessen zweites Ack bleiben gültig); REVOKED→APPROVED öffnet den
  Re-Signing-Pfad (UC-15); OFFERED→NEGOTIATION existiert doppelt — per
  Submit für den Ersteller, per Negotiate für die Gegenseite.
- **Was gilt:** Die Verhandlungs-Autorisierung teilt sich am Eigentum: Bei
  einem empfangenen (inbound) Angebot leitet sich das Verhandlungsrecht aus
  der Rolle als designierte Gegenpartei ab, nicht aus lokalem
  Negotiator-RBAC; lokales RBAC gilt nur für selbst verfasste Verträge.
- **Was gilt:** Ein struktureller Redline wird sofort auf `contract_data`
  angewandt und als neu gerendertes PDF an den Peer geshippt (ADR-13); eine
  Freitext-Änderungsanfrage wird nur für den Verhandlungs-Audit-Trail
  gespeichert. Lost-Update-Schutz vergleicht gegen `content_updated_at`,
  das nur echte Inhaltsänderungen bewegt — Zustandsübergänge und
  Hintergrund-Writes lösen keinen False-Trip aus.
- **Was gilt:** Signaturfelder werden in der Verhandlung materialisiert
  (ein `dcs:SignatureField` pro beteiligter Instanz, benannt nach deren
  DID — das AcroForm-Feld der Signier-Zeremonie, ADR-12). Eine explizite
  Deklaration gewinnt: Ein Vertrag, der bereits Felder deklariert, wird
  nie ergänzt; das Auto-Seeding ist idempotent.
- **Was gilt:** Deployment (ADR-25/ADR-27): Das Ziel ist Eigenschaft des
  Vertrags (damit der automatische Trigger nach Signatur ohne Menschen ein
  Ziel hat); ein Operator-Override lenkt nur eine Zustellung um, ändert
  aber die Designation nicht. Der Dispatch-Datensatz kopiert den
  Registry-Endpoint zum Zeitpunkt des Versands — spätere Registry-Edits
  schreiben nicht um, was ein Deployment tatsächlich tat. Der
  Payload-Content-Hash ist kanonisch (rekursiv key-sortiert, kompakt, ohne
  HTML-Escaping), damit das Zielsystem ihn aus dem geparsten JSON
  reproduzieren kann.
- **Was gilt:** Multi-Signer-Gate: Deploy verlangt, dass jedes deklarierte
  Signaturfeld signiert ist — mit Ausnahme der Felder fremder Parteien:
  eine Gegenpartei signiert in IHREM Deployment, ihr Signaturdatensatz
  erreicht diese Instanz nie; deren Signatur trägt das empfangene,
  inhaltsverifizierte Artefakt.
- **Was gilt:** Das Deployment-Callback ist die autoritative Erfolgsmeldung
  (fehlgeschlagener Outbound-Call ist nicht fatal, wird aber am
  Dispatch-Datensatz markiert und vom Compliance-Monitor alarmiert). Ein
  Callback wird nur akzeptiert, wenn der authentifizierte OAuth2-Client
  dem Registry-Eintrag des dispatchten Ziels entspricht — ein Ziel kann
  eigene Deployments quittieren und keine fremden (ADR-27; ersetzt das
  frühere „irgendein Ziel kannte das Shared Secret"). Das Ack versiegelt
  ein RFC-3161-gestempeltes Execution-Evidence-Receipt im Archiv und
  schaltet SIGNED→ACTIVE.
- **Was gilt:** Die Webhook-Plattform ist eine In-Memory-Fan-Out-Schicht:
  feste Liste abonnierbarer Events (`KnownEvents`), Mapping interner
  NATS-Event-Typen auf Webhook-Namen (`DCSEventMap`), asynchrone
  Zustellung, Delivery-Log begrenzt auf die letzten 512 Einträge (die
  Beobachtungsfläche von GET /deliveries). Abonnements überleben keinen
  Restart.

- **Was gilt:** Vertragszustände sind ein Postgres-Enum, das nur additiv
  erweitert wird (`ALTER TYPE … ADD VALUE`); Views, die neue Werte
  referenzieren, liegen in einer separaten, später sortierten Migration.
  **Warum:** Postgres erlaubt die Verwendung neuer Enum-Werte nicht in der
  Transaktion, die sie anlegt; das Enum trägt bereits Live-Spalten (ADR-2).
- **Was gilt:** `contracts_effective`-Views überschreiben den gespeicherten
  Zustand nach Ablauf von `exp_date` mit `EXPIRED`, außer der Vertrag ist
  bereits terminal (TERMINATED/REJECTED/EXPIRED/WITHDRAWN/REVOKED).
  **Warum:** Auto-Expiry über einem bereits finalen Zustand wäre irreführend.
- **Was gilt:** Deployment-Ziele (ADR-25) werden aus `CONTRACT_TARGETS_FILE`
  beim Start gesät; vorhandene Registry-Einträge werden nicht überschrieben.
  **Warum:** Eine frische Installation braucht Ziele vor dem ersten
  Admin-Login; ein Operator, der ein Ziel umkonfiguriert, darf vom nächsten
  Restart nicht überstimmt werden.
- **Was gilt:** Zeit-/Intervall-Konfiguration ist in `base/conf`
  zentralisiert; env-überschreibbare Werte (z. B.
  `DCS_SYNC_FAIL_RETRY_INTERVAL`, `DCS_ARCHIVE_EXPIRING_WINDOW_DAYS`,
  `DCS_API_RATE_LIMIT_PER_MINUTE`) fallen bei unparsbaren oder nicht
  positiven Werten stets auf den Default zurück.

## Identität (did:web, Peer-Vertrauen, Maschinen-Identitäten)

- **Was gilt:** Jede Instanz publiziert ein did:web-Dokument mit drei
  Verification Methods aus dem PKCS#11-Token: eIDAS/JAdES-Identitätsschlüssel
  (mit x5c), VC-Signaturschlüssel (eigene Methode, eigenes x5c) und
  keyAgreement-Schlüssel (ohne x5c — er signiert nie). **Warum:** Ein
  Verifier muss den Identitätsschlüssel vom VC-Schlüssel unterscheiden
  können (ADR-19, ADR-ocmw-vc-signing).
- **Was gilt:** Peer-Vertrauen ruht auf drei Schichten: (1) eIDAS-Zertifikatskette
  im DID-Dokument, validiert gegen den EU-Trust-Pool; (2) Challenge-Response
  mit dem DID-Schlüssel pro Request; (3) Federation-Trust-Gate
  (Agreement-Credential + Policy-Endpoint, ADR-19). **Warum:** Es
  gibt keine gemeinsame Auth-Autorität über unabhängige Betreiber hinweg,
  daher kein Shared Token.
- **Was gilt:** Der EU-Trust-Pool wird aus der LOTL und den nationalen
  Trusted Lists gebaut (nur CA/QC mit Status "granted"), täglich
  aktualisiert; ein fehlgeschlagener Refresh behält den letzten guten Pool.
  Selbstsignierte Zertifikate aus der übermittelten Kette werden nie als
  Trust-Anchor akzeptiert. Der Pool wird nur bei gesetztem
  `DCS_FORCE_EIDAS_CERT` gebaut; ohne ihn entfallen Kettenprüfung UND
  QCStatements-Prüfung, es bleiben Hostname- und JWK-Abgleich — das ist keine
  eIDAS-Validierung. **Warum:** QCStatements sind eine
  Selbstdeklaration des Ausstellers; rechtlich tragfähig wird die Prüfung
  erst mit den EU Trusted Lists als Anker.
- **Was gilt:** did:web-Auflösung folgt der Methodenspezifikation exakt:
  nackte Authority → `/.well-known/did.json`, Pfadsegmente → Segmente ohne
  `.well-known`. Mehrere Instanzen können sich einen Host teilen. **Warum:**
  Falsche Pfadableitung ließe jede DID unter einem Host auf dasselbe
  Dokument auflösen.
- **Was gilt:** Der keyAgreement-Schlüssel eines Peers wird als das einzige
  keyAgreement des fremden DID-Dokuments aufgelöst (mehrdeutige Dokumente
  werden abgewiesen); lokal wird per Label-Suffix aufgelöst, nie über
  Array-Positionen. **Warum:** Fremde HSM-Labels sind unbekannt; ein
  Dokument, das Methoden ergänzt oder umsortiert, muss weiter den richtigen
  Schlüssel liefern.
- **Was gilt:** Maschinen-Identitäten werden zur Request-Zeit aus der
  Registry aufgelöst; `DCS_SYSTEM_CLIENTS` ist nur ein deklarativer Seed,
  der bei jedem Start reconciled wird (ADR-27). Rollen von System-Clients
  kommen aus Konfiguration/Registry, nie aus Token-Claims.
- **Was gilt:** Der IP-Lockout zählt nur fehlgeschlagene AUTHENTIFIZIERUNG:
  Ein gültiges Token ohne die geforderte Rolle ist eine
  Autorisierungsentscheidung (403), wird als authentifiziertes
  Access-Event geloggt und zählt nicht zur Sperre — sonst sperrte sich ein
  Nutzer beim legitimen Rollenwechsel von einer IP selbst aus
  (DCS-FR-UC-01-4). Attempt-Logging und Lock-Pflege sind best-effort: ein
  DB-Problem blockiert den Login-Fluss nicht.
- **Was gilt:** Ein deaktivierter System-Client-Registry-Eintrag löst zu
  „kein Maschinen-Caller" auf statt zu einem Fehler: Das Token fällt in den
  Human-Pfad durch und wird dort abgewiesen — Deaktivieren widerruft eine
  Integration sofort, ohne auf den Ablauf ihres Secrets zu warten.
- **Was gilt:** Der Hintergrund-PDF-Regenerator authentifiziert sich mit dem
  In-Cluster-Service-Credential (`SystemToken`), das exakt die Scopes der
  intern benötigten Endpoints trägt und den Cluster nie verlässt — er läuft
  auf NATS-Events ohne Nutzer-JWT, muss aber die internen
  PKCS#11-Signaturprimitiven erreichen (DCS-IR-HI-01).
- **Was gilt:** Alle ausgehenden Fetches der Identitätsschicht (did.json
  eines Peers, x5c-Kettenglieder per URL) laufen über einen Client mit
  10-Sekunden-Timeout. **Warum:** `http.DefaultClient` hat keinen Timeout;
  ein Peer, der die Verbindung annimmt und nie antwortet, würde die
  Verifikation sonst unbegrenzt hängen lassen.

## Signaturen (HSM, JAdES, TSA)

- **Was gilt:** Sämtliche Private-Key-Operationen laufen durch das
  PKCS#11-Token (`base/hsm`); es gibt bewusst keinen Software-Fallback —
  falsche Modul-/Token-/PIN-Konfiguration verhindert, dass der Prozess
  gesund wird (DCS-NFR-SEC-02, ADR-1). Nur das Öffnen des Tokens beim Start
  wird retry-t statt sofort fatal: der HSM-Provisioner läuft als
  Post-Install-Hook und kann bei frischer Installation noch ausstehen; ein
  Bootstrap-Server belegt derweil die Listen-Adresse und liefert 503 auf
  `/readyz`.
- **Was gilt:** Fünf Schlüssel-Zwecke mit festen Default-Labels (dcs-did,
  dcs-vc, dcs-oid4vp-jar, dcs-c2pa, dcs-ecdh; je per `DCS_HSM_KEY_*`
  überschreibbar). Rotation erzeugt `-v<N>`-Labels neben dem Basislabel;
  alte Versionen bleiben im Token für die Verifikation historischer
  Signaturen (`pki_active_key_version`, DCS-OR-C2PA-007).
- **Was gilt:** Alle Schlüssel sind ECDSA P-256, alle Signaturen SHA-256.
  crypto11 liefert ASN.1 DER; JOSE (ES256) und COSE brauchen das 64-Byte
  r||s-Format — die Konvertierung ist zentral in `hsm.SignES256`.
- **Was gilt:** ECDH-Ableitung (`CKM_ECDH1_DERIVE`, CKD_NULL) öffnet eine
  eigene PKCS#11-Session neben dem crypto11-Signing-Kontext und finalisiert
  den Cryptoki-State nie. **Warum:** crypto11 exponiert keine Derive-API;
  Cryptoki-State ist prozessweit.
- **Was gilt:** Beim Start läuft ein Wrap/Unwrap-Selbsttest gegen den
  keyAgreement-Schlüssel des eigenen did.json. **Warum:** beweist, dass
  publizierter Schlüssel und HSM-Schlüssel zusammenpassen, bevor der erste
  Artefakt-CEK entsteht.
- **Was gilt:** DCS-zu-DCS-Broadcasts sind JAdES baseline-B (kompaktes JWS,
  x5c im Protected Header, kritisches `sigT`) über der JCS-kanonisierten
  Vertragsrepräsentation (RFC 8785). Der Empfänger verifiziert Signatur und
  Bindung an den did:web-Schlüssel des Senders vor Annahme (DCS-FR-SM-02).
  **Warum JCS:** bit-identisch reproduzierbar aus jeder konformen
  JCS-Implementierung in jeder Sprache.
- **Was gilt:** Timestamps kommen als RFC-3161-TSR über ORCE (kein direkter
  TSA-Kontakt); verifiziert wird gegen das einkompilierte TSA-Zertifikat
  (`certs/tsa.crt`, per `TSA_TRUST_CERT_FILE` überschreibbar — gelesen erst
  nach Laden der Env-Konfiguration, nicht im Package-Init). TSA-Requests
  sind idempotente hash-keyed GETs und werden bis zu 3× mit Backoff
  wiederholt. Providerwechsel = ORCE-Flow umstellen + Zertifikat tauschen +
  Rebuild.
- **Was gilt:** Zertifikatswiderruf des (geteilten) Dev-Signing-Zertifikats
  wird per Ops-Job (`cmd/crlcheck`) fleet-weit in
  `contract_signatures.cert_revoked_at` materialisiert, nicht per
  HTTP-Endpoint pro Request (DCS-OR-C2PA-007).

Signatur-Annahmekette (Zeremonie → Prüfung → Anwendung), Grundsatz ADR-20:

- **Was gilt:** Eine PAdES-Signatur setzt eine verifizierte Signier-Zeremonie
  voraus: Das Wallet präsentiert die PID direkt über OID4VP (EUDIPLO ist
  keine Abhängigkeit); die Präsentation wird gegen die Nonce der Zeremonie
  und die konfigurierten Trust-Anchors kryptografisch verifiziert, bevor sie
  persistiert wird — der Persistenz-Handler selbst verifiziert nichts mehr.
  Eine abgelaufene Zeremonie (`ceremonyTTL`) verifiziert nie; der Retry ist
  eine frische Zeremonie.
- **Was gilt:** Eine gültige Power of Attorney ist harte Voraussetzung
  (UC-14, FR-SM-03) und muss die tatsächlich signierte Partei autorisieren:
  Das Signaturfeld trägt die Organisations-DID, die PoA-Organisation muss
  ihr entsprechen.
- **Was gilt:** Bei jedem Prepare werden die exakten To-be-signed-Bytes
  (PDF + kanonisches JAdES-Payload) samt Finalize-Metadaten gepinnt; Submit
  validiert gegen die committeten Bytes statt sie neu abzuleiten. Ebenso
  gepinnt: die vertragseigene Signaturstufen-Anforderung des Feldes
  (`dcs:requiredCredentialType`, Default AES) — Submit gated darauf, nie
  auf den Caller-Parameter.
- **Was gilt:** Ein publizierter Signing-Request ist single-use: Der
  Konsum-Marker (`consumed_at IS NULL`-Guard) committet in derselben
  Transaktion wie das Finalize — zwei konkurrierende Callbacks können nie
  beide finalisieren.
- **Was gilt:** Der EU-DSS ist ein ZUSÄTZLICHER externer AdES-Validator
  neben den internen Prüfungen; konfiguriert-aber-unerreichbar ist ein
  Fehler, nie ein Skip. AES-Akzeptanz verlangt bewusst NICHT DSS
  TOTAL-PASSED: TOTAL-PASSED setzt eine qualifizierte EU-Trust-List-CA
  voraus (eine QES-Eigenschaft); AES braucht nur Integrität und eindeutige
  Signatar-Bindung (eIDAS Art. 26) — ein INDETERMINATE mit reiner
  Trust-/POE-Lücke wird akzeptiert, jeder Krypto-/Integritätsfehler nicht.
  Subject/Serial des Signier-Zertifikats werden als
  Sole-Control-Nachweis am Zeremonie-Datensatz festgehalten (DCS-FR-SM-26).
- **Was gilt:** Der Verify-Endpoint meldet den PDF-Signatur-Check als
  `not_available` statt „passed": Der Pfad re-verifiziert die PAdES-CMS-
  Signatur über ihrer /ByteRange nicht kryptografisch, und ein nicht
  durchgeführter Check darf nie als bestanden gemeldet werden
  (DCS-OR-C2PA-006).

## Dokumente und Provenance (Artefakt-Speicherung, Verschlüsselung, Löschung)

Grundsatzentscheidung Verschlüsselung/Key-Shredding: ADR-28. Die
Detail-Entscheidungen der Umsetzung:

- **Was gilt:** Jedes Artefakt wird vor IPFS mit einem per-Scope zufälligen
  CEK (AES-256-GCM) verschlüsselt; der Scope-String ist als GCM-AAD in jeden
  Ciphertext gebunden. Scopes: `contract:<IRI>` (die Art.-17-Löscheinheit:
  PDFs, Archiv-Snapshot, Audit-Bodies), `template:<IRI>`, `instance:<DID>`
  (Checkpoints, Reports — nicht shredbar).
- **Was gilt:** Der CEK existiert at rest ausschließlich als gewrappter
  Datensatz in `content_encryption_keys` (ECDH-ES+A256KW auf den
  keyAgreement-P-256-Schlüssel: ephemerer Sender-Keypair → ECDH →
  Concat-KDF → RFC-3394 Key Wrap). Wrappen braucht nur den Public Key (pure
  Go); Unwrappen läuft durch das HSM. Klartext-CEKs leben nur in einem
  begrenzten In-Memory-Cache (256 Einträge), der beim Shred geleert wird.
- **Was gilt:** Shredding markiert alle Live-Records des Scopes als
  zerstört (`shredded_at/by/reason`) — der Datensatz bleibt als
  Zerstörungsnachweis stehen, wird nie hart gelöscht. Ein geshreddeter
  Scope liefert nie wieder einen Schlüssel, auch nicht für Writes, und wird
  durch Peer-Ships nicht wiederbelebt. **Warum:** DCS-NFR-COMP-03/SEC-13 —
  Löschung muss endgültig und nachweisbar sein; Lesepfade melden einen
  `ShreddedError` (4xx, nie 500).
- **Was gilt:** CEK-Insert und Shred serialisieren über einen
  Postgres-Advisory-Lock auf dem Scope; der Shred läuft in derselben
  Transaktion wie das `KEY_SHREDDED`-Audit-Event. **Warum:** Nach einem
  Shred darf nie wieder ein Live-Record entstehen; Zerstörung und
  Audit-Nachweis committen atomar.
- **Was gilt:** Für die Föderation wird der CEK eines Scopes an den
  keyAgreement-Schlüssel des Peers gewrappt und reist im Ship-Payload; der
  Empfänger unwrapped mit dem eigenen HSM, re-wrapped an den eigenen
  Schlüssel und persistiert idempotent (ein vorhandener eigener Live-Record
  gewinnt). Die lokale Empfänger-Zeile dokumentiert, welche fremden
  Instanzen den CEK halten — die Basis des Peer-Erase-Handshakes.
- **Was gilt:** IPFS-Zugriff läuft zweigleisig: Tenant-Store (XFSC) plus
  synchron gepinnte Kubo/MFS-Kopie. Lesen versucht erst einmal den Tenant,
  dann sofort Kubo, erst danach den Tenant-Retry mit Backoff. **Warum:** Der
  Tenant-Store ist eventually consistent und verliert unter Last transient
  DataIdentifier-Mappings; der Audit-Chain-Walk darf weder 404en noch in
  den Request-Deadline laufen — die Kubo-Kopie ist content-addressed
  identisch, die Hash-Kette verifiziert weiter. `files/cp`-Kollisionen
  (Peer hat denselben CID schon kopiert) gelten nach CID-Vergleich als
  Erfolg.

- **Was gilt:** Ein PDF, das bereits eine PAdES-Signatur trägt, wird nie
  wieder durch das C2PA-Lifecycle-Stamping geschickt: Jedes inkrementelle
  Update an einem referenzierten Embedded-File-Objekt nach der Signatur —
  auch byte-range-erhaltend — werten standardkonforme PAdES-Validatoren als
  unerklärte Modifikation. Der „active"-Zustand wird deshalb VOR dem
  Signieren gestempelt (update-then-sign); ab Nicht-„draft"-C2PA-Zustand ist
  das PDF eingefroren.
- **Was gilt:** Vertrags-PDFs werden nie on-demand erzeugt: Jede inhalts-
  oder lebenszyklusrelevante CWE-Transition (inkl. Update) triggert die
  Hintergrund-Regeneration, die den C2PA-Manifest-Chain fortschreibt.
- **Was gilt:** Der Bundle-Export (ZIP) re-nutzt ausschließlich bestehende
  Retrieval-Pfade und paketiert die lokal bekannte Hierarchie-Familie:
  Parent-Kette rekursiv aufwärts unter `parents/`, übrige lokal bekannte
  Familienmitglieder flach unter `related/` — gefiltert durch dieselbe
  Party-Read-Scoping-Regel wie ein Direktabruf (ADR-7: nur was der
  Anfragende ohnehin einzeln lesen dürfte). Nicht-lokale Mitglieder fehlen
  einfach; nichts wird remote geholt. Ein struktureller Integritäts-
  Pre-Flight (FR-PACM-06) verweigert den Export mit Findings-Liste (422),
  statt ein unvollständiges ZIP zu liefern; `bundle-manifest.json` indiziert
  jeden Eintrag mit SHA-256.
- **Was gilt:** Archiv-Löschung (DCS-FR-CSA-17) ist Soft-Delete (Zeile
  bleibt als Nachweis) plus harte Disposal-Kette: Snapshot-CIDs werden vor
  dem Soft-Delete gesichert (danach gäbe es keinen Nachweis mehr, welche
  IPFS-Objekte betroffen sind), die Snapshots verlassen den IPFS-Store,
  anschließend läuft die CEK-Erasure lokal plus Peer-Handshake. Der
  Erasure-Status ist abfragbar (live/geshreddet lokal, pro Peer
  pending/confirmed).

### Rendering und Provenance-Einbettung (pdf-core)

pdf-core ist bewusst eine simple, zustandslose, SCHLÜSSELLOSE Komponente
(Projektinvariante): sie hält nie Signaturmaterial und trifft keine
Fachentscheidungen — sie rendert, bettet ein und extrahiert.

- **Was gilt:** Das Rendering ist deterministisch: gleiche JSON-LD-Payload →
  byte-identisches PDF. Dazu ist die Render-Epoche fixiert
  (`CanonicalCompiledAt`; die vertrauenswürdige Vertragszeit ist der
  PAdES-B-T-Timestamp, nicht die Render-Epoche), und die sichtbare Seite wird
  ausschließlich aus der `@list`-geordneten `documentStructure` der Payload
  abgeleitet — ohne json-gold-Expand/Compact-Roundtrip, der
  Mehrfachwerte nicht-deterministisch umsortiert. **Warum:** /verify beweist
  „das menschenlesbare Dokument IST der Render seiner maschinenlesbaren
  Payload" durch Recompile-und-Byte-Vergleich (DCS-OR-C2PA-002/-010).
- **Was gilt:** Die Payload wird VERBATIM eingebettet: exakt die
  übermittelten Bytes, nie eine re-kanonisierte Form. FileID, gerenderter
  Backlink und CIDv1-Content-Address sind sha256 über genau diese Bytes.
  **Warum:** Nur ein Byte-Commitment ist von jedem Verifier reproduzierbar;
  URDNA2015-Graph-Hashing ist für die Selbstreferenz ungeeignet
  (Blank-Node-Labeling ist nicht byte-deterministisch, A- und B-Instanz
  errechneten verschiedene Hashes).
- **Was gilt:** C2PA-Signieren ist zweistufig prepare/embed: der Render
  signiert mit einem CapturingSigner (zeichnet jede COSE-Sig_structure auf,
  hinterlässt genullte 64-Byte-Slots), das DCS-Backend signiert die
  Sig_structures mit dem dcs-c2pa-HSM-Schlüssel und postet sie an das
  zustandslose `/c2pa/embed`, das nur Byte-Runs füllt. pdf-core kennt nur
  die x5chain (Public-Material) für den Protected Header. **Warum:**
  Schlüsselmaterial bleibt vollständig im Backend/HSM (DCS-IR-HI-01); embed
  hängt von nichts als seinen Inputs ab, jede Replika kann es bedienen.
  Die ES256-Signaturen sind der einzige nicht-deterministische Teil eines
  PDFs; `ZeroCOSESignatures` maskiert sie für alle Byte-Vergleiche.
- **Was gilt:** Änderungen sind Amend-statt-Rerender: jede inhaltliche
  Änderung ist ein PDF-Inkremental-Update, das die Originalbytes als Präfix
  unangetastet lässt und Objekte (Pages, Payload, C2PA-Manifest)
  supersedet. **Warum:** Bestehende C2PA-Hard-Bindings und eine ggf. schon
  angebrachte PAdES-Signatur (/ByteRange) bleiben über den Originalbytes
  verifizierbar; die C2PA-Manifest-Kette wächst pro Hop weiter.
  `/verify` re-appliziert deterministisch die GESAMTE Hop-Historie
  (Basis-Compile plus jedes Update), nicht nur den letzten Schritt.
- **Was gilt:** Über einem PAdES-signierten PDF ist ein Update
  provenance-only: Seiten, AcroForm und das signierte Feld (mit /V-Link)
  werden nie neu emittiert, /Root bleibt auf dem Katalog des Signers.
  **Warum:** Ein Re-Stamping würde das /V des signierten Feldes verwerfen
  und die Signatur invalidieren (DCS-OR-C2PA-010).
- **Was gilt:** Re-Anchor-Ausnahme (ADR-26): Die PAdES-Signatur wird NACH
  dem Lifecycle-Manifest angebracht (sie committet damit auf die
  Provenance), wodurch das Whole-File-Binding des Manifests kürzer ist als
  die signierte Datei. `/render/reanchor` hängt deshalb ein provenance-only
  Manifest mit unveränderter Payload an — der einzige Pfad, der den
  No-Changes-Guard bewusst passiert; /verify erkennt einen solchen Hop am
  unveränderten Payload und replayed ihn als Re-Anchor.
- **Was gilt:** Interop-Entscheidungen gegen c2pa-rs/c2patool: Amendments
  sind Standard-Manifeste (c2ma), nie C2PA-Update-Manifeste (c2um), deren
  Parent-Binding nach einem Append nie mehr matchen kann. Manifest-Labels
  werden aus dem Hard-Binding-Hash (nicht dem Payload-Hash) abgeleitet,
  damit zwei Amendments derselben Payload keine zyklische
  Ingredient-Kette erzeugen. Remote-Manifest-Discovery läuft als eigene
  `dcs.remote_manifests`-Assertion plus XMP-`dcterms:provenance`-Link —
  NICHT als Claim-Feld, das c2pa-rs als unbekannt hart abweist. Das
  Leaf-Zertifikat der x5chain MUSS eine organizationName tragen, sonst
  melden alle C2PA-Verifier claimSignature.mismatch — pdf-core macht das
  zum Startup-Konfigurationsfehler.
- **Was gilt:** Tamper-Evidenz im Verify: Eine unsignierte Basis muss
  byte-GLEICH ihrem Recompile sein — angehängte Bytes ohne
  dcs-Update-Marker sind ein Offline-Amendment außerhalb von /update und
  werden abgewiesen (409). `/verify/content` prüft dagegen NUR die
  Seiteninhalte gegen den Recompile der eingebetteten Payload, tolerant
  gegenüber angehängten Manifest-/Signatur-Schichten — die Prüfung, die
  eine Instanz auf ein peer-empfangenes, bereits amendetes PDF anwendet.
- **Was gilt:** Eine Invariante wird selbst überwacht: Kein Byte eines
  Seiteninhalts-Streams darf je in einem C2PA-Exclusion-Fenster liegen
  (sonst wäre sichtbarer Inhalt unprovenanced); Verletzung ist ein
  Compiler-Bug und panict bzw. schlägt die Verifikation explizit fehl.
- **Was gilt:** Die assertierende Instanz (DID) reist als per-Request-Header
  (`X-DCS-Lifecycle-Authority`) in jede `dcs.lifecycle`-Assertion und wird
  beim Verify aus dem Dokument selbst zurückgelesen. **Warum:** pdf-core ist
  ein zustandsloser Renderer, den mehrere Instanzen teilen können; der
  Recompile eines fremden Dokuments muss ohne Vorwissen dieselben Bytes
  reproduzieren.
- **Was gilt:** PDF/A-3a-Konformität ist einkompiliert: eingebettetes
  Font-Programm (Liberation Sans), synthetisches sRGB-ICC-Profil im
  OutputIntent, /ID in jedem Trailer (auch inkrementell), gelistete
  Associated Files (/AF) für jede Attachment-Beziehung, XMP mit
  pdfaid-Deklaration und PDF/A-Extension-Schema für dcterms.
- **Was gilt:** Die JSON-LD-Kanonisierung am Rand (`CanonicalizePayload`)
  kompensiert json-gold-Verhalten explizit: Ohne API-level Base-IRI würden
  relative `@id`s still verworfen (Payload-Hash über null Quads); `@container:
  @list`/`@type:@id` aus dem Kontext greifen bei Compact-IRI-Schreibweise
  nicht und werden nachgezogen. Der registrierte Kontext wird in-process
  aufgelöst — Kanonisierung und SHACL-Validierung machen keinen
  HTTP-Fetch.

## Web-Client (Frontend)

- **Was gilt:** Sitzungen ruhen auf dem Hydra-Access-Token in localStorage;
  beim Reload wird die Identität (sub, Rollen, Issuer) direkt aus dem
  gespeicherten, ungelaufenen Token rehydriert statt via `/auth/refresh`.
  **Warum:** Hydra rotiert Refresh-Tokens single-use — ein Reload darf kein
  Refresh-Token verbrauchen, nur um die Identität wiederherzustellen.
- **Was gilt:** Eine Sitzung ist nur mit mindestens einer gemappten
  DCS-Rolle gültig: Rollen kommen ausschließlich aus den JWT-Claims und
  werden auf das UserRole-Vokabular gemappt; ein Token ohne mappbare Rolle
  stellt keinen User her. Der Router gated Routen deklarativ über
  Rollen-Metadaten.
- **Was gilt:** Der OID4VP-Login bindet Hydras login_challenge über einen
  sessionStorage-Zustandsautomaten (`hydra-login-guard`) idempotent an die
  laufende Präsentation (Challenge kann vor oder nach dem
  Präsentations-State eintreffen — pending-Latch); consent_challenge wird
  direkt an das Backend weitergereicht, die Challenge-Query anschließend
  aus der URL entfernt. Alle Hydra-/OIDC-Pfade laufen über den
  Origin-Proxy — der Browser braucht keine direkte Hydra-Adresse.
- **Was gilt:** Der Template-/Vertrags-Editor (`dcsDraftStore`) hält das
  Dokument DIREKT in kanonischem JSON-LD (Blocks, Layout, ContractFields,
  contractData, ODRL-Regeln als JSON-LD-Knoten); das abgelegte Dokument ist
  eine triviale Zusammenstellung dieses Zustands, es gibt keine
  Konvertierungsschicht. **Warum:** Was der Editor bearbeitet, ist exakt
  das, was Backend-Validierung und pdf-core konsumieren — kein
  Format-Drift zwischen UI-Modell und Dokument.
- **Was gilt:** Komponenten werden flatten-on-compose eingefügt: Blocks,
  Placeholder und ODRL-Regeln einer approbierten Komponente werden mit
  frischen `@id`s (alle internen Referenzen konsistent umgeschrieben)
  in das Zieldokument kopiert — kein Referenzblock, kein Snapshot; das
  Dokument bleibt selbst-enthalten (ADR-23) und zwei Einfügungen derselben
  Komponente kollidieren nie.
- **Was gilt:** Eine maschinenlesbare Regel überlebt ihre Prosa nie: jede
  ODRL-Regel trägt `dcs:prose` auf die tragende Klausel; das Löschen der
  Klausel entfernt die Regel, und eine Regel ohne bindenden Klauseltext
  wird schon beim Autorisieren abgewiesen. Client-seitig wird das einzelne
  umschließende `odrl:Offer` assembliert (ADR-6-Kanonform).
- **Was gilt:** Fehler der Signatur-Endpoints werden über den typisierten
  Goa-Fehlernamen (`name`-Feld) auf Nutzertexte gemappt, nie über
  String-Matching der freien `message` — die ist kein stabiler Vertrag.
- **Was gilt:** Die Signier-UI erzwingt die ADR-20-Byte-Pinning-Kopplung:
  prepare und submit reisen mit derselben `ceremony_id`; das zu signierende
  Feld ist der Party-Slot der EIGENEN Instanz (identifiziert über deren
  did:web, nicht über den Issuer des eingeloggten Nutzers — der benennt die
  Organisation des Signatars und matcht nie einen Party-Slot).

## Föderation (DCS-zu-DCS)

- **Was gilt:** Das Föderations-Regelwerk (`federation/rules.md`) ist per
  `go:embed` in das Binary kompiliert; jede Agreement-Credential benennt es
  per öffentlicher Policy-URL und SHA-256-Hash (ADR-19). **Warum:** Zwei
  Instanzen desselben Builds betten byte-identische Regeln ein und errechnen
  denselben Hash; ein manipuliertes oder anders versioniertes Regelwerk
  fällt beim Hash-Vergleich auf.
- **Was gilt:** Die Agreement-Credential wird beim Start einmal gebaut und
  selbst signiert (Issuer = eigene Instanz-DID, ecdsa-rdfc-2019); die
  DCS-eigenen Terme sind explizit im JSON-LD-Kontext gemappt. **Warum:**
  RDFC-1.0-Kanonisierung würde unmapped Terme sonst stillschweigend fallen
  lassen — die Signatur deckte sie nicht ab.
- **Was gilt:** Zuverlässige Zustellung von Contract-Ships kommt vom
  DB-gestützten Sync-Fail-Scheduler (`sync_fails`-Tabelle, Default 5 min),
  nicht vom NATS-Event-Bus. **Warum:** NATS liefert at-most-once; die
  Reconciliation muss deutlich innerhalb einer Verhandlungsrunde laufen.
- **Was gilt:** Geshippt werden genau die Zustände OFFERED, NEGOTIATION,
  SIGNED und REVOKED (ADR-13; Revocation muss sofort propagieren,
  DCS-NFR-BR-06); interne Zustände (DRAFT, SUBMITTED, REVIEWED, APPROVED,
  ACTIVE, TERMINATED) bleiben lokal — Review/Approval überqueren nie die
  Grenze. Das PDF ist das Wire-Format (JSON-LD + C2PA-Kette + Signaturen);
  ein signierter Vertrag trägt zusätzlich die JAdES.
- **Was gilt:** Ein Offer ist ein reiner Zustandsübergang ohne neues PDF
  (DRAFT und OFFERED mappen beide auf C2PA „draft"), daher triggert das
  Offer-Event selbst den Ship des bereits gespeicherten PDFs. Ist der
  Vertrag shipbar, das PDF aber noch nicht gespeichert (asynchrone
  Regeneration), wird nie still verworfen: Es entsteht ein
  `sync_fails`-Eintrag für den Retry-Scheduler.
- **Was gilt:** Trust-Gate-Semantik im Sync (ADR-19): Ein Agreement-
  Credential-Fehler (Layer 3a) wird via `sync_fails` retry-t, mit
  Incident-Dedup über den `gate_incident_recorded`-Latch der Zeile (genau
  ein Incident pro Eintrag, unabhängig von Interleavings). Eine
  Policy-Endpoint-Ablehnung (Layer 3b) ist terminal: kein Retry-Eintrag,
  vorhandene Retry-Einträge werden atomar mit dem Incident gelöscht,
  Incidents dedupen pro (Vertrag, Peer, Richtung) — Offer und
  PDF_REGENERATED derselben Offerte feuern Millisekunden auseinander.
- **Was gilt:** Peer-Authentifizierung im Ship ist ein Body-Level
  did:web-Challenge-Response (Zufallswert, signiert mit dem eigenen
  DID-Schlüssel), kein JWT. **Warum:** Es gibt keine gemeinsame
  End-User-Identität über unabhängig betriebene Instanzen.
- **Was gilt:** Der Empfänger übernimmt das geshippte PDF EXAKT als seine
  Kopie (nie regeneriert), damit die C2PA-Provenance-Kette der Gegenseite
  erhalten bleibt; die lokale Kopie wird aus dem per pdf-core extrahierten
  JSON-LD upserted und landet in NEGOTIATION mit eigenen lokalen Tasks
  (ADR-13: jede Instanz eigenes Workflow/RBAC). Einzige Ausnahme der
  Intrinsic-State-Privatheit: Ein als REVOKED deklarierter Ship wird als
  Revocation übernommen — der Widerruf der Gegenseite voidet das Agreement
  unabhängig vom lokalen Workflow-Fortschritt.
- **Was gilt:** Die Peer-Erasure läuft über eine eigene
  `contract_erasures`-Queue mit Retry-Scheduler (gleiche Mechanik wie
  sync_fails, getrennte Queue): lokaler Shred mit KEY_SHREDDED-Event ist
  harter Fehler, unzustellbare Peer-Requests bleiben pending. Die
  Partei-Auflösung läuft vor jeder Zerstörung — nichts wird zerstört, wenn
  der Vertrag nicht lesbar ist.

## Audit (Tamper-Evidence, Outbox, Merkle-Checkpoints)

Grundsatzentscheidung Checkpoints/externe Verankerung: ADR-16. Die
Detail-Entscheidungen:

- **Was gilt:** Audit-Logging läuft als transaktionale Outbox: Domain-Events
  werden in der Business-Transaktion in `outbox_events` persistiert; ein
  asynchroner Prozessor publiziert sie auf NATS und verankert sie separat in
  IPFS/TSA. Publishing (`published`, 100-ms-Takt) ist bewusst vom Anchoring
  (`processed`, 1-s-Takt) entkoppelt. **Warum:** Subscriber konsumieren nur
  das JSON-Payload, nie einen Anchor-Wert — Publishing darf nicht hinter den
  sequenziellen, netzgebundenen TSA/IPFS-Roundtrips warten.
- **Was gilt:** Pro Ressource bilden Einträge eine strikte Hash-Kette
  (Verweis auf den Vorgänger-CID); global committet ein Merkle-Checkpoint
  pro Batch (RFC-6962-Domain-Separation, Root-Kette über `prev_root`,
  RFC-3161-Timestamp auf dem Root). **Warum:** Die frühere globale
  Per-Event-Kette zwang jedes Event durch einen sequenziellen
  TSA+IPFS-Roundtrip und ließ ein hängendes Event den ganzen Trail stauen.
- **Was gilt:** Die TSA-Signatur ist nicht Teil der Checkpoint-Bytes; ein
  Checkpoint ohne Timestamp wird gespeichert und später nachgestempelt.
  **Warum:** Der Root ist immutabel — der Timestamp attestiert nur "existierte
  spätestens zu diesem Zeitpunkt"; ein TSA-Ausfall darf den Trail nicht
  blockieren.
- **Was gilt:** Jeder Eintrag trennt öffentlichen Header (Komponente,
  Event-Typ, DID, Timestamps, Kettenlink, Nonce) vom privaten Body:
  `event_data` liegt verschlüsselt unter dem CEK-Scope des Eintrags. Ein
  Shred löscht den Body, ohne ein gespeichertes Byte zu bewegen — Kette und
  Inclusion-Proofs verifizieren weiter; Leser sehen den definierten
  Erased-Marker (`{"erased":true}`). Erasure-Records selbst
  (KEY_SHREDDED, DELETE_ARCHIVED) liegen im nicht-shredbaren Instanz-Scope,
  obwohl sie die Vertrags-DID tragen: sie müssen die Löschung überleben,
  die sie dokumentieren.
- **Was gilt:** Jeder Merkle-Leaf ist durch eine 16-Byte-Nonce geblendet.
  **Warum:** Ohne Salz wäre der Leaf-Hash ein Commitment über hochgradig
  erratbaren Inhalt; ein publizierter Proof erlaubte Brute-Force-Bestätigung
  von Kandidaten-Einträgen.
- **Was gilt:** Jeder geschriebene Eintrag/Checkpoint wird sofort über den
  Lesepfad rückverifiziert, bevor er Kettenlink wird; permanent
  unverankerbare Events werden nach 50 Versuchen dead-lettered (sichtbar
  geloggt — sie fehlen dann im Trail). RETRIEVE_*/SEARCH_*-Events werden
  publiziert, aber nie verankert. **Warum:** Lookup-Traces würden die
  Batches dominieren und echte Audit-Events aushungern; Audit-Reads filtern
  sie ohnehin aus (`IsAuditVisibleEventType`).
- **Was gilt:** Trail-Reads laufen chain-parallel (unabhängige
  Ressourcen-Ketten nebeneinander, Limit 128; Checkpoint-Leaves Limit 32)
  und komplett-Trail-Reads über maximal 500 Checkpoints rückwärts.
  **Warum:** O(Events) sequenzielle IPFS-Roundtrips sprengen jede
  Request-Deadline, sobald der Trail Historie hat.

- **Was gilt:** Das Compliance-Monitoring kennt vier Risikoklassen:
  MISSING_APPROVAL (Vertrag in approval-pending-Zustand mit offener
  Pflicht-Approval-Task), UNAUTHORIZED_ACCESS (persistiertes
  CONTRACT_ACCESS_DENIED-Artefakt des Party-Read-Scopings),
  CONTRACT_UNDERPERFORMANCE (vom Zielsystem gemeldeter KPI-Wert, der die
  vertragseigene ODRL-SLA-Constraint verletzt) und CONTRACT_DEPLOYMENT_FAILED
  (Dispatch, den das Zielsystem nie erreichte). Risiken werden pro
  (Vertrag, Akteur) bzw. (Vertrag, Approver) dedupliziert; der Sweep selbst
  wird als Audit-Event verankert, jedes Risiko zusätzlich an der
  PAC-Kette des betroffenen Vertrags.
- **Was gilt:** Der Monitor MELDET persistierte Urteile, er bildet keine
  zweiten: KPI-Verletzungen bewertet der Deployment-Callback bei Eingang
  gegen die Vertrags-ODRL und persistiert das Verdikt; Deployment-Fehler
  markiert der Deploy-Pfad. **Warum:** Eine Re-Evaluation könnte divergieren;
  bei Underperformance wären die unveränderlichen Inline-Vertragswerte
  ohnehin nie verletzt (ein sie verletzender Vertrag wäre nie approved
  worden) — nur gemeldete Ist-Werte können alarmieren. Monitoring blockiert
  nicht: Der Vertrag ist in Kraft, die Verletzung ist ein zu
  dokumentierender Fakt (im Gegensatz zum Approval-Gate, das verweigert).

## Betrieb (Startup, HTTP, Konfiguration)

- **Was gilt:** Startreihenfolge: Migrationen → Semantic-Hub-Seed +
  Anker-Refresh → Bootstrap-Server (503 auf /readyz) → HSM-Open mit Retry →
  DID-Selbsttest → Config-Attestation → Worker/Services → Handover der
  Listen-Adresse an den echten Server. Helm läuft bewusst ohne `--wait`
  (HSM-Provisioner ist Post-Install-Hook).
- **Was gilt:** Sicherheitskritische gemountete Konfigurationsdateien
  (DID-Dokument, OID4VP-Trust-Daten, x5c-Anker) werden beim Start gehasht,
  gegen Operator-Pins (`DCS_CONFIG_SHA256_PINS`) geprüft und als
  Attestation in die Audit-Outbox geschrieben; Pin-Mismatch bricht den
  Start ab (DCS-NFR-SEC-04).
- **Was gilt:** `DCS_PUBLIC_URL` ist Pflicht und die Basis jeder absoluten
  IRI, die produzierte Dokumente tragen (Ressourcen-IRIs, `@context`,
  `sh:shapesGraph`, C2PA-Remote-Manifeste) — sie müssen für externe
  Konsumenten dereferenzierbar sein. Ressourcen-Identität ist einheitlich
  `{DCS_PUBLIC_URL}/<kind>/<key>` (`base.ResourceIRI`); bereits absolute
  Identifier (IRI, did:, urn:) passieren unverändert.
- **Was gilt:** Externe Pflicht-Abhängigkeiten (pdf-core, Status-List-
  Service) werden beim Start bis zu 3 Minuten lang gepollt statt sofort zu
  crashen. **Warum:** Unter CI-Ressourcendruck sind Abhängigkeiten oft nur
  langsam, nicht kaputt; ein Crash-Loop wäre teurer als Warten.
- **Was gilt:** did.json wird an der Origin-Root gemountet (did:web
  well-known), außerhalb des konfigurierbaren API-Prefixes (`DCS_API_PATH`);
  ORCE-Webhooks unter `/orce/*` vor dem Goa-Mux; Frontend-Statics mit
  index.html-Fallback unter `DCS_UI_PATH`.
- **Was gilt:** Fehler-Mapping am HTTP-Rand: benannte Goa-ServiceErrors
  werden auf die korrekten Statuscodes gemappt;
  `BundleExportRefusedError` wird vor der generischen Heuristik behandelt,
  damit das `findings`-Array im Response-Body erhalten bleibt (422).
- **Was gilt:** Migrationen sind eingebettete, alphabetisch sortierte
  SQL-Dateien, je in eigener Transaktion, getrackt in `schema_migrations`;
  es gibt keine Down-Migrationen.
