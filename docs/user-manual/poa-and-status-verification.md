# PoA- und Statusprüfung bei Signatur und PDF-Verifikation

## Nutzen

Der DCS lässt eine Signatur für eine Organisation nur zu, wenn die vorgelegte
Vertretungsvollmacht (PoA) zur signierenden Person und Organisation passt,
aktuell gültig ist und auf einen für die jeweilige Umgebung zugelassenen
Vertrauensanker zurückführt. Bei Verträgen zwischen zwei DCS-Instanzen prüft
die empfangende Instanz diesen Nachweis erneut.

Die PDF-Verifikation zeigt den Vertragslebenszyklus und den aktuell abgefragten
Sperrstatus getrennt. Dadurch bleibt beispielsweise „beendet“ als
Vertragszustand (`terminated`) sichtbar, während der Live-Status zugleich
„gesperrt“ (`revoked`) meldet.

## Voraussetzungen und Rollen

- Die signierende Person (Contract Signer) benötigt eine gültige Identitäts-
  und Vertretungsvollmacht in ihrer Wallet.
- Die Vertretungsvollmacht muss für die signierende Organisation ausgestellt
  sein und einen Statusnachweis enthalten.
- Die Umgebung muss den Aussteller und dessen Vertrauensanker ausdrücklich
  zulassen. Die Demo verwendet dafür ausschließlich ihr Entwicklungsprofil;
  Produktionsvertrauen wird vom Betreiber konfiguriert.
- Vertragsmanager (Contract Manager) können einen Vertrag beenden, eine
  Signatur widerrufen und anschließend den exportierten Vertrag verifizieren.

## Signieren

1. Starten Sie den Signaturvorgang für den freigegebenen Vertrag.
2. Bestätigen Sie in der Wallet die Präsentation der Identitäts- und
   Vertretungsvollmacht.
3. Der DCS prüft Inhaberbindung, Organisation, Vertrauenskette und den aktuellen
   Status.
4. Nur bei erfolgreicher Prüfung wird die Signatur fortgesetzt. Bei einem
   Vertrag zwischen zwei DCS-Instanzen wiederholt die empfangende Instanz die
   Prüfung vor der Annahme.

## Vertragsstatus veröffentlichen und prüfen

1. Beenden Sie als Vertragsmanager den Vertrag oder widerrufen Sie eine
   Signatur. Geben Sie beim Signaturwiderruf einen nachvollziehbaren Grund ein
   und bestätigen Sie den Vorgang.
2. Die Statusänderung wird dauerhaft zur Veröffentlichung vorgemerkt und bei
   vorübergehenden Fehlern erneut versucht.
3. Exportieren und verifizieren Sie den Vertrag.
4. Prüfen Sie im Ergebnis getrennt:
   - den Lebenszyklusstatus, zum Beispiel `active`, `suspended` oder
     `terminated`;
   - den Live-Sperrstatus `active` oder `revoked`;
   - das Ergebnis der Live-Prüfung (`passed` oder `failed`) und gegebenenfalls
     den Fehlergrund.

Eine bestätigte Sperre oder Suspendierung wird innerhalb von fünf Minuten bei
einer frischen Prüfung berücksichtigt.

## Sichtbare Fehlerfälle

- **Nicht vertrauenswürdige Kette:** Die Vertretungsvollmacht wird abgelehnt;
  der Signaturvorgang bleibt unverifiziert.
- **Falsche Person oder Organisation:** Die Signatur beziehungsweise die
  Annahme durch die Gegenstelle wird abgelehnt.
- **Widerrufene oder suspendierte Vollmacht:** Die Signatur wird nicht
  fortgesetzt. Im binären Demo-/XFSC-Statusmodell werden beide Fälle als
  gesetzte Sperre behandelt.
- **Fehlender Widerrufsgrund:** Ein leerer oder nur aus Leerzeichen bestehender
  Grund wird nicht akzeptiert; die Signatur bleibt unverändert.
- **Fehlender, unbekannter oder ungültiger Statusnachweis:** Die Prüfung wird
  aus Sicherheitsgründen abgelehnt.
- **Statusdienst nicht erreichbar:** Die PDF-Verifikation meldet den
  Live-Status als `unavailable`, die Statusprüfung als `failed` und zeigt einen
  Fehlergrund. Das Gesamtergebnis ist nicht erfolgreich und darf den Vertrag
  nicht als live `active` ausweisen.
- **Abgelehnter Nachweis einer Gegenstelle:** Die Gegenstelle speichert keine
  Signaturprovenienz für die abgelehnte Annahme; der Ablehnungsgrund bleibt als
  prüfbarer Befund erhalten.

## Geltungsbereich

Für die aktuelle Abgabe ist eine direkte PoA-Kette vom ausstellenden Zertifikat
zum Vertrauensanker vorgesehen. Das Entwicklungsvertrauen der Demo ist kein
Produktionsstandard. Auswahl, Betrieb und Pflege des Produktions-Trustprofils
liegen beim Betreiber.
