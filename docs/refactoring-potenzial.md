# Refactoring-Potenzial

Beobachtungen aus dem Kuratier-Durchgang. Nichts hiervon wurde umgesetzt —
das Dokument sammelt Kandidaten für bewusst entschiedene Refactorings.
Risikoeinschätzung: niedrig / mittel / hoch (Risiko der Änderung, nicht des
Ist-Zustands).

Stand dieser Fassung: Backend-Kern (`backend/internal/base/`, `backend/cmd/`,
`backend/migrations/`), Backend-Fachschichten (`backend/internal/` übrige
Pakete, `backend/design/`), `pdf-core/`, `frontend/ClientApp/src/`,
`deployment/`.

## Vermutlich tot, nicht beweisbar / nur test-referenziert

- **`backend/internal/base/tsa/tsa.go` — Paket-Level `Verify`:** kein
  Produktionsaufrufer (Produktion nutzt `(*APIClient).Verify`); wird aber
  von vier Tests direkt aufgerufen, darunter der
  FreeTSA-Integrationstest. Vorschlag: Tests auf `client.Verify` umstellen
  und die Paketfunktion entfernen, oder als bewusste Test-/Tooling-API
  belassen. Risiko: niedrig.
- **`backend/internal/base/validation/remoteshapesource.go` —
  `VerifyAgainstOriginatorHub`:** kein Produktionsaufrufer; ADR-23
  beschreibt es als Peer-seitige Verifikation, der frühere BDD-Docstring
  („called from post_sync") behauptete einen Aufruf, der im Code nicht
  existiert — der Docstring wurde in Etappe 3 korrigiert (beschreibt jetzt
  nur die Reachability-Voraussetzung). Offen bleibt die Verhaltensfrage:
  Verifikation im Sync-Pfad anbinden oder Funktion entfernen.
  Risiko: mittel (Verhaltensfrage, nicht Codefrage).
- **`backend/internal/base/hsm/config.go` — `VersionedLabel`:** nur vom
  eigenen Unit-Test referenziert. Die Schlüsselrotation
  (`pki_active_key_version`, key_inventory) komponiert Labels derzeit nicht
  über diese Funktion. Anbinden oder entfernen. Risiko: niedrig.
- **`backend/internal/base/datatype/userrole/userrole.go` —
  `IsSystemRole`/`IsHumanRole`:** nur vom eigenen Test referenziert;
  dokumentieren aber die Rollensemantik (SRS Tabelle 5). Entfernen oder in
  der Autorisierung tatsächlich nutzen. Risiko: niedrig.
- **`backend/migrations/sql/20260703_create_kv_store.sql` — Tabelle
  `kv_store`:** das einzige konsumierende Paket (`base/kv`) war unreferenziert
  und wurde in diesem Durchgang gelöscht; die Tabelle bleibt als verwaistes
  Schema (Migrations sind tabu). Kandidat für eine spätere Drop-Migration.
  Risiko: niedrig.

- **`backend/internal/signingmanagement/pidverify/pidverify.go` — `Verify`
  (und Helfer `kbJWTNonce`):** kein Go-Aufrufer irgendwo; genutzt wird aus
  dem Paket nur die `Audience`-Konstante (ceremony.go,
  service/signature_request.go). Der Paket-Docstring beschreibt eine
  Re-Verifikation der PID-SD-JWT+KB-JWT-Präsentation — klären, ob diese
  Prüfung im Zeremonie-Pfad anderweitig abgedeckt ist (oid4vp.Verifier)
  oder tatsächlich fehlt; dann anbinden oder Paket auf die Konstante
  eindampfen. Risiko: mittel (sicherheitsrelevante Verhaltensfrage).
- **`backend/internal/middleware/oidc.go` —
  `InjectBearerToken`/`GetBearerToken`:** der Mechanismus ist write-only:
  drei Inject-Stellen (auth/jwt_auth.go, pdfgeneration/event/subscriber.go),
  aber kein einziger Leser — `GetBearerToken` ist tot, der pdf-core-Client
  sendet keinen Authorization-Header. Entweder die Token-Weiterleitung an
  pdf-core tatsächlich bauen oder das Paar samt Inject-Aufrufen entfernen.
  Risiko: niedrig-mittel.
- **`backend/internal/auth/oid4vp/status/envelope/` — Signier-Seite nur
  test-referenziert:** `SignCOSEVC`, `SignStatusListCWT`,
  `SignDataIntegrityCredential`, `ECDSASigner`, `Ed25519Signer` samt
  COSE-Encode-Helfern haben keinen Produktionsaufrufer — die Produktions-DCS
  verifiziert Status-Listen/Credentials nur (die Signier-Seite ist der
  externe Status-List-Service); die Tests nutzen sie als Fixture-Bau-API.
  Als bewusste Test-API markieren oder in ein Testpaket verschieben.
  Risiko: niedrig.
- **`backend/internal/pdfgeneration/pdfcore/client.go` — `New`:** nur
  test-referenziert; Produktion nutzt `NewWithAuthority`. Tests umstellen
  und entfernen. Risiko: niedrig.
- **`backend/internal/service/archive_integrity_audit.go` —
  `archiveIntegrityRuleForError`, `audit_report.go` —
  `reportDownloadEnvelope`:** nur von Tests aufgerufen. Prüfen, ob die
  Produktionspfade sie nutzen sollten oder die Tests Totes testen.
  Risiko: niedrig.
- **`Scan`/`Value` auf Workflow-Datatypes:** deadcode meldet
  `Scan`/`Value` von `ApprovalTaskState`, `ContractTemplateState`,
  `NegotiationTaskState`, `ReviewTaskState`, `NegotiationDecision`,
  `SigningStatus` (contractworkflowengine und templaterepository) als
  unerreichbar — database/sql ruft sie aber per Reflection, RTA sieht das
  nicht zuverlässig. Nicht gelöscht. Prüfen, welche dieser Typen wirklich
  in gescannten DB-Structs vorkommen; nicht vorkommende Implementierungen
  entfernen. Risiko: mittel (Reflection-Beweis nötig).
- **Nur-empfangene Event-Structs mit Interface-Methoden:** `EventType()`/
  `GetDID()` von `RemoteActionRequestEvent`, `RemoteSyncEvent`,
  `RemoteSyncRequestEvent`, `RecoverOutdatedPeerEvent`, `OutdatedPeerEvent`,
  `VerifyEvent` (contractworkflowengine/event) und `SearchEvent`
  (signingmanagement/event) sind unerreichbar — diese Events werden nur
  deserialisiert, nie über `event.Create` publiziert. Methoden dienen nur
  der Interface-Symmetrie; klären, welche Events überhaupt noch entstehen.
  Risiko: niedrig.
- **`eventtype.SigningRequest`/`Revoke` ohne Emitter:** der einzige Emitter
  von SIGNING_REQUEST (`signingmanagement/command/signingrequest.go`) war
  tot und wurde in diesem Durchgang gelöscht; die Enum-Konstante bleibt.
  `cwe eventtype.Revoke` dokumentiert selbst „no command emits it yet".
  Kandidaten für Bereinigung, sobald die Event-Taxonomie konsolidiert wird.
  Risiko: niedrig.

- **`pdf-core/compiler/update.go` — `UpdatePDFWithVC`, `claim.go` —
  `StripEmbeddedJSONLD`:** keine Produktionsaufrufer; `UpdatePDFWithVC` wird
  nur von Unit-Tests genutzt (Produktion geht durch `UpdatePDFWithOptions`),
  `StripEmbeddedJSONLD` nur von Tests/BDD-Steps als Tamper-Fixture. Als
  bewusste Test-API markieren oder Tests umstellen und entfernen.
  Risiko: niedrig.
- **`deployment/helm/templates/pdf-core-signing-secret.yaml` —
  `signer-key-pem`:** der Secret-Eintrag wird von keinem Template gemountet
  und von pdf-core nicht gelesen (pdf-core ist keyless; konsumiert wird nur
  die x5chain). Leftover des früheren In-Process-Signers; Secret-Feld und
  `pdfCore.signing.signerKeyPEM`-Values-Pfad sind Kandidaten für eine
  Chart-Bereinigung (Deployment-Löschungen in diesem Durchgang tabu).
  Risiko: niedrig.
- **Frontend — nur intern genutzte Exporte:** u. a.
  `hydra-login-guard.ts` (`bindLoginChallengeOnce`, `loginChallengeFromURL`,
  `stripHydraChallengeQuery`), diverse nur als Rückgabetyp dienende
  `export interface`s in `src/services/*` und `src/models/*`. Funktional in
  Benutzung, aber die `export`-Fläche ist größer als der Konsum; bei einer
  API-Straffung `export` entfernen. Risiko: niedrig.

## Duplikation

- **DER→r||s-Konvertierung dreifach:** `hsm.ECDSADERToRaw`,
  `jades.derToJOSE` und implizit `hsm.SignES256` implementieren dieselbe
  Konvertierung. Vorschlag: `jades` auf `hsm.ECDSADERToRaw` umstellen.
  Risiko: niedrig.
- **JWK→ECDSA-PublicKey-Dekodierung doppelt:**
  `identity.PublicKeyJWK.ECPublicKey` und `envelope.EphemeralPublicKey.publicKey`
  bauen beide P-256-Keys aus x/y-Base64; envelope validiert zusätzlich
  On-Curve, identity nicht. Vereinheitlichen (inkl. On-Curve-Check).
  Risiko: niedrig.
- **`cmd/dcs/dotenv.go`:** `loadDotenvIfPresent` und `loadDotenvFile` sind
  bis auf den Pfad identisch (ersteres ist `loadDotenvFile(".env")`).
  Risiko: niedrig.
- **`cmd/dcs/http.go`:** 13 nahezu identische Mount-Log-Schleifen über die
  Server-Mounts; eine Helper-Funktion über ein Slice von Servern würde ~40
  Zeilen sparen. Risiko: niedrig.
- **`base/audittrail.go`:** `ReadAuditLogEntriesByComponentAndDID` und die
  Chain-Walk-Closure in `ReadAuditLogEntriesByComponent` duplizieren den
  identischen Ketten-Walk (fetch → decode → predecessor). Risiko: niedrig.
- **`gendid`/`identity`:** `cmd/gendid/main.go#didWebHost` dupliziert
  `identity.DIDWebToHostname`. Risiko: niedrig.

- **`datatype/`-Pakete doppelt zwischen contractworkflowengine und
  templaterepository:** `approvaltaskstate`, `contracttemplatestate`,
  `reviewtaskstate`, `actionflag`, `contracttemplatetype` existieren in
  beiden Paketen nahezu identisch; auch das `eventtype`-Muster ist fünffach
  kopiert (auth, cwe, pac, tci, tr, sm). Ein gemeinsames
  Enum-Hilfsmuster/gemeinsame Pakete würden die Kopien eindampfen.
  Risiko: mittel (breite Import-Umstellung, DB-Enum-Kopplung beachten).
- **`Responsible` doppelt:** `contractworkflowengine/db` und
  `signingmanagement/db` definieren je einen eigenen Responsible-Typ für
  dieselbe jsonb-Spalte. Risiko: niedrig.
- **did:web-Peer-Client-Konstruktion dreifach:** `synchronizer.shipToPeers`,
  `eraser.SendErase` (und httpclient-Umfeld) wiederholen identisch
  `DIDWebPath` → Prefix-Join → `NewDCSToDCSHttpClient`; ein
  `clientForPeer(did)`-Helper genügt. Risiko: niedrig.
- **`templatecatalogueintegration/internal/ptr`:** generische
  Pointer-/Map-Helfer, die es sinngemäß auch in `base` gibt; nach der
  Löschung zweier ungenutzter Funktionen bleiben drei kleine Helfer, die
  konsolidierbar wären. Risiko: niedrig.

- **pdf-core — fünffacher Filespec→Stream-Walker:** `extractJSONLDStream`,
  `extractEmbeddedStreamByFileSpecName`, `findEmbeddedJSONLDStreamRange`,
  `ExtractEmbeddedVC` und `ExtractSigningEvidence` implementieren denselben
  Ablauf (Filespec-Marker → /EF-Referenz → Objekt → stream/endstream) je
  eigenständig. Ein gemeinsamer Helfer (Parameter: Dateiname,
  first/last-Occurrence) genügt. Risiko: niedrig.
- **pdf-core — CBOR-Text-Decoder doppelt:** `parseCBORTextMap`/
  `decodeCBORText` existieren nahezu identisch in `compiler/compiler_c2pa.go`
  und `manifest/cbor.go` (manifest-Variante kann zusätzlich 16-Bit-Längen).
  Risiko: niedrig.
- **pdf-core — Appendix-Builder dreifach:** `buildUpdateAppendixBytes`,
  `buildSignedUpdateAppendixBytes` und `buildVerificationAppendixBytes`
  wiederholen Objekt-Emission, xref-Subsection-Schreiber und
  Trailer-Assembly; auch `findObjectStreamRange`/`findLastObjectStreamRange`
  sind bis auf first/last identisch. Risiko: niedrig-mittel (byte-genaue
  Determinismus-Pfade, Tests decken sie aber dicht ab).
- **Frontend — Modul- vs. Legacy-Baumstruktur:** Views/Stores existieren
  teils unter `src/modules/<modul>/…` (per Alias importiert), teils unter
  `src/views/`/`src/stores/`; die in diesem Durchgang gelöschten Dateien
  unter `src/views/template-repository/` waren veraltete Kopien der
  Modul-Views. Eine Konvention (alles modulweise) würde künftige
  Kopien-Drift verhindern. Risiko: niedrig (reine Verschiebung).

## Inkonsistente Muster / Schichtfragen

- **`base/ipfs/ipfs.go` — `DeleteFile` (Tenant-Pfad):** ignoriert den
  HTTP-Status der Antwort vollständig (kein `resp.StatusCode`-Check) und
  gibt bei 4xx/5xx `nil` zurück; der Kubo-Pfad (`deleteKuboFile`) prüft
  korrekt. Außerdem unpinnt DeleteFile nur den Tenant ODER Kubo, nie beide —
  eine über `copyToMFS` erzeugte Pinned-Kopie überlebt ein Tenant-Delete.
  Risiko: mittel (Verhaltensänderung im Löschpfad).
- **`base/ipfs/ipfs.go` — HTTP-Client-Nutzung gemischt:** `getOnce` nutzt
  `http.Get` (DefaultClient ohne Timeout) statt des konfigurierten
  `c.client` mit 10-s-Timeout. Risiko: niedrig.
- **`base/hsm/ecdh.go` — Session pro Aufruf:** `DeriveECDH` lädt Modul,
  sucht Slot, öffnet Session und loggt sich pro Unwrap neu ein. Bei
  CEK-lastigen Pfaden (Audit-Read über viele Einträge mildert der
  CEK-Cache) potenziell teuer; eine gehaltene Session wäre schneller, aber
  Cryptoki-State-Management ist heikel. Risiko: mittel.
- **`base/event/outboxprocessor.go` — Ticker ohne ctx:** die drei
  Scheduler-Loops (`for range ticker.C`) beachten `ctx.Done()` nicht; die
  Goroutinen leben bis zum Prozessende. Konsequent wäre `select` auf ctx
  wie in `EUTrustPool.StartAutoRefresh`. Risiko: niedrig.
- **`base` (Wurzelpaket) mischt Belange:** Audit-Trail-Reader, Merkle,
  Ressourcen-IRIs, UUIDs und Env-Helper in einem Paket, auf das fast alles
  zeigt. Aufteilen (z. B. merkle/ und auditread/) würde Abhängigkeiten
  schärfen. Risiko: mittel (breite Import-Umstellung).
- **`base/db` + `db/pq` — Durchreiche-Schicht:** `db/outbox.go` sind reine
  1:1-Weiterleitungen an `pq.Postgres*`-Funktionen; die Audit-Seite nutzt
  dagegen ein Interface. Ein Muster wählen. Risiko: niedrig.
- **`cmd/dcs/db.go`:** `NewDatabaseConnection` gibt `(db, error)` zurück,
  ruft bei Verbindungsfehler aber selbst `log.Fatalln` — der Fehlerpfad des
  Aufrufers ist toter Code. Risiko: niedrig.

- **Ticker ohne ctx auch in dcstodcs:** `startSyncFailScheduler` und
  `StartEraseRetryJob` laufen als `for range ticker.C` ohne
  `ctx.Done()`-Select — gleiche Beobachtung wie bei
  `base/event/outboxprocessor.go`. Risiko: niedrig.
- **`backend/internal/webhookplatform` — reine In-Memory-Plattform:**
  Subscriptions, Pending-Callbacks und Delivery-Log überleben keinen
  Restart; für produktive Integrationen (DCS-FR-CSA-20, DCS-FR-TR-22) wäre
  eine DB-Persistenz konsequent. Risiko: mittel (bewusste Betriebsfrage).

## Überlange Funktionen

- **`cmd/dcs/main.go#main` (~700 Zeilen):** komplette
  Dependency-Verdrahtung in einer Funktion. Abschnitte (HSM/Identity,
  IPFS/Artifacts, Eventing, Services) in Konstruktor-Hilfsfunktionen
  gliedern. Risiko: niedrig-mittel (nur Bewegung, aber viel davon).
- **`base/validation/documentdata.go` (~1100 Zeilen) und
  `contractcontentaudit.go` (~870 Zeilen):** je mehrere kohärente
  Teilbereiche (Envelope-Normalisierung, ODRL-Shape-Validierung,
  Graph-Validierung) in einer Datei; Dateisplit ohne API-Änderung möglich.
  Risiko: niedrig.
- **`signingmanagement/command/apply.go` (~1480 Zeilen) und
  `service/contract_workflow_engine.go` (~1840 Zeilen):** die beiden
  größten Fachdateien; apply.go bündelt Signatur-Anwendung,
  PDF-Freeze-Logik und JAdES-Recovery, der CWE-Service alle
  Endpoint-Adapter. Dateisplit entlang der bestehenden Abschnitte möglich.
  Risiko: niedrig.

## Sonstiges

- **`base/utils.go#GetEnvOrDefault`:** Typ-Switch-Generik über
  `string | bool` — funktioniert, aber `strconv`-basierte, getrennte Helfer
  wären einfacher. Risiko: niedrig.
- **`base/id.go`:** `GenerateID` liefert `*string` (Pointer auf frischen
  Wert) — Aufrufer dereferenzieren durchgängig sofort; ein Wert-Return wäre
  idiomatischer. Risiko: niedrig (breite, mechanische Anpassung).
- **pdf-core — PDF-Parsing per Byte-Scan:** der gesamte Compiler liest und
  patcht PDFs über `bytes.Index`-Marker-Scans statt eines PDF-Parsers —
  bewusst simpel und für selbst erzeugte PDFs ausreichend, aber jede
  Formatänderung (z. B. Objekt-Header-Layout) muss in vielen Scannern
  synchron nachgezogen werden. Nur beobachtet, kein Handlungsbedarf.
- **pdf-core — Embed-Snapshot `pdf-core/docs/` veraltet stumm:** die
  SHACL-/Kontext-Artefakte werden per `go:embed` aus dem gitignorierten
  Snapshot `pdf-core/docs/` gelesen, den `make -C pdf-core docs` als
  `cp -r ../docs docs` befüllt. Das Make-Target erkennt Änderungen am
  Quellbaum nicht zuverlässig („up to date" trotz neuer Shapes) — ein
  veralteter Snapshot lässt die svc-/compiler-Tests mit
  SHACL-ClassConstraint-Fehlern (`dcs:layout`) fehlschlagen, obwohl der
  Code korrekt ist. Vorschlag: Target mit echter Abhängigkeit auf den
  Quellbaum (oder stets phony + rsync) und ein Hinweis im README.
  Risiko: gering; Verwechslungsgefahr mit echten Regressionen hoch.
