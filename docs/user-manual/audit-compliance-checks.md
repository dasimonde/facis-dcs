# Audit- und Compliance-Prüfungen ausführen

Mit dem Auditing Tool prüfen Sie Vorlagen, Verträge, Signaturen oder
Archivvorgänge. Die Anwendung sammelt die zur gewählten Prüfung gehörenden
Nachweise und lässt die endgültigen Feststellungen durch den für Ihre Umgebung
eingerichteten Prüfdienst erzeugen. Die mitgelieferte Umgebung verwendet dafür
ORCE; der Betreiber kann einen kompatiblen eigenen Prüfdienst konfigurieren.

## Voraussetzungen und Rollen

- Sie sind als **Auditor** angemeldet.
- Für einen auf ein einzelnes Objekt begrenzten Lauf ist dessen Kennung
  verfügbar.
- Der Betreiber hat einen erreichbaren Prüfdienst und die dafür geltenden
  Regeln eingerichtet.
- Ein **Archive Manager** kann ausschließlich den Bereich **Archiv** prüfen und
  daraus Berichte exportieren.

## Prüfung ausführen

1. Öffnen Sie das **Auditing Tool**.
2. Wählen Sie den Bereich **Vorlagen**, **Verträge**, **Signaturen** oder
   **Archiv**.
3. Grenzen Sie die Prüfung bei Bedarf auf ein bestimmtes Objekt ein.
4. Geben Sie eine nachvollziehbare Begründung für die Prüfung ein.
5. Starten Sie die Prüfung.
6. Prüfen Sie die angezeigten Feststellungen. Jede Feststellung nennt die
   angewendete Regel, das Ergebnis und den Grund. Eine erfolgreiche Prüfung
   darf auch eine leere Feststellungsliste liefern.

Eine gestartete Prüfung wird genau einmal an den eingerichteten Prüfdienst
übergeben. Bei einem technischen Fehler erfolgt kein automatischer
Wiederholungsversuch und es werden keine ersatzweise lokal erzeugten
Feststellungen angezeigt. Starten Sie einen neuen Lauf erst, nachdem die
Ursache behoben wurde.

Bei Signaturprüfungen werden nur die für die Nachweisführung erforderlichen
Metadaten übergeben, beispielsweise Status, Zeitpunkte, Prüfsummen und
Referenzen. Rohe Signaturdaten und JAdES-Tokens werden nicht an den Prüfdienst
weitergegeben.

## Bericht exportieren

1. Führen Sie zuerst eine erfolgreiche Prüfung für den gewünschten Bereich und
   gegebenenfalls das gewünschte Objekt aus.
2. Wählen Sie im Auditing Tool **Bericht exportieren**.
3. Wählen Sie **JSON**, **CSV** oder **PDF**.
4. Geben Sie die Begründung für den Export ein und laden Sie den Bericht
   herunter.

Der Bericht wird ausschließlich aus dem zuletzt passend gespeicherten
Prüflauf erzeugt; der Prüfdienst wird beim Export nicht erneut aufgerufen.
JSON enthält das unveränderte gespeicherte Prüfergebnis. CSV und PDF stellen
denselben Lauf in einem anderen Format dar. Der exportierte Inhalt wird mit
seiner Prüfsumme und, sofern eingerichtet, einer Archiv-Referenz im Audit Trail
nachgewiesen.

## Prüfungen bei Vertragsübergängen

Beim Einreichen, Anbieten, Genehmigen, Signieren und Bereitstellen eines
Vertrags wird automatisch der zum Vertrag gespeicherte Regelstand geprüft. Sie
müssen dafür keinen separaten Prüflauf starten.

- **Bestanden:** Der angeforderte Übergang wird fortgesetzt.
- **Manuelle Prüfung erforderlich:** Der Vertrag bleibt im bisherigen Zustand.
  Ein **Compliance Officer** prüft den gespeicherten Lauf und genehmigt oder
  verwirft ihn mit einer Begründung. Nach einer Genehmigung wird genau der
  zurückgestellte Übergang fortgesetzt; der externe Prüfdienst wird dafür nicht
  erneut aufgerufen.
- **Blockiert:** Der Vertrag bleibt im bisherigen Zustand. Ein neuer Versuch ist
  erst sinnvoll, nachdem der angezeigte Regelverstoß oder technische Fehler
  behoben wurde.

Während eine manuelle Prüfung offen ist, darf der Vertragsinhalt oder
Vertragszustand nicht geändert werden. Andernfalls wird die Fortsetzung
abgelehnt, damit eine Entscheidung nicht auf einen inzwischen geänderten
Vertrag angewendet wird.

## Sichtbare Fehlerfälle

- **Nicht angemeldet oder nicht berechtigt:** Melden Sie sich mit der Rolle
  **Auditor** an. Ein Archive Manager darf nur den Archivbereich verwenden.
- **Ungültiger Bereich oder fehlende Begründung:** Korrigieren Sie die Auswahl
  beziehungsweise ergänzen Sie eine Begründung; die Prüfung wurde noch nicht
  an den Prüfdienst übergeben.
- **Prüfdienst nicht erreichbar oder Zeitüberschreitung:** Der Lauf ist
  technisch fehlgeschlagen. Es gibt keinen automatischen zweiten Versuch und
  keinen lokalen Ersatzbefund.
- **Ungültige Antwort des Prüfdienstes:** Version, Zuordnung oder
  Feststellungsformat stimmen nicht mit dem erwarteten Prüfauftrag überein.
  Der Lauf wird abgelehnt und nicht als erfolgreiches Ergebnis gespeichert.
- **Kein passender gespeicherter Prüflauf:** Für Bereich und Objekt existiert
  noch kein erfolgreicher Lauf. Führen Sie zunächst die Prüfung aus.
- **Nicht unterstütztes Berichtsformat:** Wählen Sie JSON, CSV oder PDF.
- **Bericht konnte nicht archiviert werden:** Der Export wird als Fehler
  gemeldet, statt einen nicht nachgewiesenen Bericht auszugeben.
- **Regelstand nicht auflösbar:** Eine für diesen Vertrag gespeicherte Regel-
  oder Profilversion fehlt oder ist nicht erreichbar. Der Übergang bleibt
  blockiert; es wird nicht auf einen anderen Regelstand ausgewichen.
- **Prüfung des Vertragsübergangs nicht erreichbar, zu langsam oder ungültig:**
  Der Übergang bleibt nach einem Prüfauftrag blockiert. Es erfolgt kein
  automatischer zweiter Auftrag.
- **Vertrag während einer manuellen Prüfung geändert:** Die frühere Genehmigung
  wird nicht auf den geänderten Vertrag angewendet. Fordern Sie den Übergang
  für den aktuellen Vertragsstand erneut an.
- **Fortsetzung bereits in Bearbeitung:** Eine weitere Genehmigung startet
  keine parallele Fortsetzung. Bleibt dieser Zustand nach einem harten
  Systemabbruch bestehen, muss der Betreiber den Vorgang prüfen.
