# Entscheidungsvorlage: Systemsignaturen, DCS-zu-DCS-Vertrauensanker, Contract Target System

Stand: 26.07.2026 — ENTWURF zur internen Prüfung, Abgabe an den Auftraggeber bis 28.07.2026.
Bezug: Entscheidungsmemo des Auftraggebers vom 26.07.2026, Punkt „DCS-SYS-SGN /
DCS-zu-DCS-Vertrauensanker / Contract Target System".

Diese Vorlage beschreibt drei Punkte, für die bisher Arbeitsannahmen galten,
und bittet jeweils um eine Entscheidung. Fachbegriffe werden bei der ersten
Verwendung kurz erklärt.

---

## 1. Rechtliche Einordnung der Systemsignatur (DCS-SYS-SGN)

**Worum geht es?** Das System versieht Dokumente an mehreren Stellen
automatisch mit kryptografischen Nachweisen — zum Beispiel, um zu belegen,
dass ein archiviertes Dokument nicht nachträglich verändert wurde. Diese
automatischen Nachweise erstellt eine Maschine, kein Mensch.

**Einordnung (im Repository dokumentiert als ADR-17):** Die eIDAS-Verordnung
definiert in Art. 3 Nr. 9 den „Unterzeichner" ausdrücklich als **natürliche
Person**. Eine elektronische Signatur — auch eine fortgeschrittene nach
Art. 26 — kann daher nur ein Mensch erstellen, und zwar mit Mitteln unter
seiner **alleinigen Kontrolle**. Was eine Maschine ohne handelnden Menschen
erzeugt, ist rechtlich keine Signatur; das Gegenstück für Organisationen ist
das **elektronische Siegel** (Art. 3 Nr. 25–27), das Unversehrtheit und
Herkunft belegt, aber keine Willenserklärung trägt.

**Was das System deshalb tut:** Die Maschinen-Rolle „System Contract Signer"
hat **keinerlei Signatur-Berechtigung** — alle signaturerzeugenden
Schnittstellen sind für Maschinen gesperrt (ADR-17, mit automatisierten
Tests abgesichert). Willenserklärungen entstehen ausschließlich durch
wallet-basierte AES-Signaturen natürlicher Personen. Die automatischen
Integritätsnachweise des Systems (Archiv-Nachweise, Zeitstempel,
Prüfketten) werden als **technische Integritätsnachweise** geführt und
nirgends als Signatur bezeichnet. Ein späterer Ausbau zu einem förmlichen
elektronischen Siegel einer juristischen Person ist als Folgearbeit
beschrieben (ADR-21 „System Contract Sealer"), aber nicht Teil von v1.

**Erbetene Entscheidung:** Bestätigung dieser Einordnung — Systemnachweise
sind technische Integritätsnachweise, keine Signaturen; die Bezeichnung
„System Contract Sealer" ersetzt perspektivisch den irreführenden
SRS-Begriff „System Contract Signer".

---

## 2. Vertrauensanker zwischen zwei DCS-Instanzen (konfigurierbare Trust-List)

**Worum geht es?** Wenn zwei Vertragsparteien je eine eigene DCS-Instanz
betreiben, müssen die Systeme entscheiden, welchem Gegenüber sie vertrauen,
bevor Verträge ausgetauscht werden.

**Arbeitsannahme (im Repository dokumentiert als ADR-19):** Vertrauen ist in
drei getrennte Schichten gelegt:

1. **Identität** — das Gegenüber weist seine Identität über eIDAS-konforme
   Zertifikate und seine veröffentlichte Systemadresse (did:web) nach.
2. **Regelakzeptanz** — das Gegenüber legt ein maschinenprüfbares
   „Föderations-Beitrittszeugnis" vor (ein signiertes Credential, das
   bestätigt, dass es sich den Regeln der Föderation unterworfen hat).
3. **Lokale Richtlinie** — jeder Betreiber führt eine **konfigurierbare
   Vertrauensliste** und kann Interaktionen zusätzlich über einen eigenen
   Richtlinien-Endpunkt zulassen oder ablehnen. Abgelehnte Zugriffe werden
   protokolliert und als Vorfall geführt.

Es gibt also keine zentrale, fest verdrahtete Vertrauensstelle: Die
Governance liegt bei den Betreibern der Föderation, die Technik erzwingt
Identitätsnachweis und Regelakzeptanz.

**Erbetene Entscheidung:** Bestätigung, dass eine je Betreiber
konfigurierbare Vertrauensliste (statt einer zentralen Zulassungsstelle) die
Governance-Anforderungen des Vorhabens trägt.

---

## 3. Contract Target System: ein Zielsystem je Vertrag

**Worum geht es?** Nach Abschluss wird ein Vertrag an ein „Zielsystem"
übergeben, das ihn technisch vollzieht (Deployment) und über das die
Vertragserfüllung gemessen wird (z. B. Kennzahlen/SLA-Werte).

**Arbeitsannahme:** **Ein Zielsystem je Vertrag.** Gründe:

- Eindeutigkeit: Jede Vollzugsmeldung, jede Kennzahl und jeder
  Prüfpfad gehört genau einem Vertrag und genau einem Zielsystem —
  Verantwortlichkeiten bleiben nachvollziehbar.
- Prüfbarkeit: Die Nachweiskette (Vollzugsempfang, Zeitstempel,
  Archiv-Eintrag) bleibt einfach und lückenlos belegbar.

Mehrere Zielsysteme bleiben abbildbar, ohne die Annahme aufzugeben: Das
System unterstützt Vertragshierarchien (Rahmenvertrag mit Teilverträgen);
je Teilvertrag kann ein eigenes Zielsystem stehen.

**Erbetene Entscheidung:** Bestätigung der Annahme „ein Zielsystem je
Vertrag", mit Vertragshierarchien als vorgesehenem Weg für Szenarien mit
mehreren Zielsystemen.

---

## Zusammenfassung der erbetenen Entscheidungen

| Nr. | Punkt | Vorschlag |
|---|---|---|
| 1 | Systemsignatur | Technischer Integritätsnachweis, keine Signatur; Signaturen nur durch natürliche Personen (wallet-basierte AES) |
| 2 | Vertrauensanker | Konfigurierbare Vertrauensliste je Betreiber, dreischichtiges Vertrauensmodell |
| 3 | Zielsystem | Ein Zielsystem je Vertrag; mehrere Zielsysteme über Vertragshierarchien |
