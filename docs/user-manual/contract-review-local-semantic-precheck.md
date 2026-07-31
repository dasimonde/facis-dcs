# Vertrag mit lokaler semantischer Vorprüfung weiterleiten

## Nutzen

Beim Weiterleiten eines Vertrags zur Freigabe prüft die Vertragsansicht zuerst
die im Vertrag hinterlegten Werte. Feststellungen werden gesammelt in einem
Dialog angezeigt, damit sie vor der Freigabe behoben werden können.

Die lokale Vorprüfung ist eine frühe Hilfestellung. Sie ersetzt nicht die
vollständigen Richtlinien- und Strukturprüfungen, die beim Weiterleiten des
Vertrags maßgeblich bleiben.

## Voraussetzungen und Rollen

- Sie sind als **Contract Reviewer** angemeldet.
- Der Vertrag befindet sich im eingereichten Zustand und wartet auf die
  Prüfung.

## Vertrag prüfen und weiterleiten

1. Öffnen Sie den eingereichten Vertrag in der Vertragsprüfung.
2. Wählen Sie **Approve**. Die lokale semantische Vorprüfung startet
   automatisch.
3. Prüfen Sie das Ergebnis:
   - Bei Feststellungen zeigt der Dialog alle gefundenen Punkte. Beheben Sie
     diese im Vertrag, bevor Sie ihn erneut weiterleiten.
   - Bei **keine Findings** können Sie einen optionalen Kommentar für die
     freigebende Person eingeben.
4. Wählen Sie **Confirm approval**, um den Vertrag weiterzuleiten.

Mit **Cancel** oder der Escape-Taste schließen Sie den Dialog, ohne eine
Entscheidung zu übermitteln.

## Sichtbare Fehlerfälle

- **Feststellungen gefunden:** Eine Bestätigung ist nicht verfügbar. Beheben
  Sie alle angezeigten Punkte und starten Sie die Freigabe erneut.
- **Lokale Vorprüfung fehlgeschlagen:** Der Dialog zeigt einen technischen
  Fehler und bietet **Retry verification** an. Der Vertrag wurde noch nicht
  weitergeleitet.
- **Weiterleitung fehlgeschlagen:** Die Vorprüfung war erfolgreich, aber die
  Oberfläche konnte die Weiterleitung nicht abschließen. Wählen Sie **Retry
  submission**; Ihr optionaler Kommentar bleibt im Dialog erhalten.
- **Approve ist nicht verfügbar:** Prüfen Sie, ob Sie als Contract Reviewer
  angemeldet sind und der Vertrag noch auf die Prüfung wartet.
