# Federated Catalogue bereitstellen

Mit dem Federated Catalogue können freigegebene Vertragsvorlagen veröffentlicht
und von Vertragserstellern gefunden werden. Die Bereitstellung meldet erst dann
„bereit“, wenn der Katalog nicht nur gestartet ist, sondern eine semantische
Prüfung erfolgreich ausgeführt hat.

## Voraussetzungen und Rollen

- Sie verfügen über Administrationsrechte für den Ziel-Namensraum.
- Helm und `kubectl` erreichen den Kubernetes-Cluster.
- Sie besitzen eine Umgebungsdatei mit den Adressen und Zugangsdaten Ihrer
  Installation.
- Für die Nutzung eines entfernten Katalogs dürfen alle beteiligten
  Installationen derselben vertrauenswürdigen Administrationsgrenze angehören.
  Der derzeitige technische Katalogbenutzer besitzt umfassende
  Administratorrechte und bietet keine Mandantentrennung.

## Betriebsart wählen

Wählen Sie genau eine der folgenden Varianten:

1. **Ohne Katalog:** Deaktivieren Sie die Katalogintegration. Die Anwendung
   startet dann ohne Katalogprüfung; Veröffentlichen und Katalogsuche stehen
   nicht zur Verfügung.
2. **Eigener Katalog:** Aktivieren Sie die Integration und die mitgelieferte
   Katalogbereitstellung. Dies ist die empfohlene Variante, wenn der Katalog
   nicht bereits zentral betrieben wird.
3. **Entfernter Katalog:** Aktivieren Sie nur die Integration und tragen Sie
   Katalogadresse, Identitätsdienst und Zugangsdaten ein. Bestätigen Sie die
   Administrator-Vertrauensgrenze nur, wenn der entfernte Katalog nicht von
   gegenseitig unvertrauten Mandanten geteilt wird.

## Bereitstellen

1. Prüfen Sie die Umgebungsdatei auf vollständige Katalogadresse,
   Identitätsdienst-Adresse, Client-ID und Client-Geheimnis.
2. Verwenden Sie das mitgelieferte Helm-Bereitstellungsskript mit Ihrer
   Umgebungsdatei, dem Ziel-Namensraum und dem Release-Namen.
3. Beobachten Sie die Ausgabe. Bei einem eigenen Katalog werden nacheinander
   Identitätsdienst, Katalog und Anwendung geprüft.
4. Die Bereitstellung ist abgeschlossen, sobald das Skript
   **Deployed and verified** meldet.
5. Veröffentlichen Sie anschließend eine registrierte Vertragsvorlage und
   rufen Sie den Vorlagenkatalog auf. Die erste Verwendung benötigt keinen
   Aufwärmlauf.

## Upgrade einer alten Entwicklungsinstallation

Eine alte, noch Neo4j/n10s verwendende Entwicklungsinstallation wird mit
dem aktuellen Helm-Release vollständig durch den Fuseki-basierten Katalog
ersetzt. Daten des alten Entwicklungskatalogs werden nicht migriert.

1. Sichern Sie Inhalte, die Sie außerhalb der Entwicklungsumgebung noch
   benötigen.
2. Führen Sie das normale Bereitstellungsskript mit demselben Release-Namen
   und Namensraum aus.
3. Prüfen Sie danach, dass `fc-fuseki` läuft und keine Neo4j-/n10s-Arbeitslast
   mehr vorhanden ist.

## Sichtbare Fehlerfälle

- **Hinweis auf `acknowledgeAdminAllTrustBoundary`:** Ein entfernter Katalog
  wurde ohne ausdrückliche Bestätigung der gemeinsamen Administrationsgrenze
  konfiguriert. Verwenden Sie einen eigenen Katalog oder prüfen und bestätigen
  Sie die Vertrauensgrenze.
- **Unvollständige Katalogkonfiguration:** Die Meldung nennt die fehlenden
  Angaben. Ergänzen Sie alle genannten Werte; eine teilweise Konfiguration
  wird nicht gestartet.
- **Realm provisioning Job failed:** Die Einrichtung des Identitätsdienstes
  ist endgültig fehlgeschlagen. Verwenden Sie den direkt ausgegebenen Grund
  und die Job-Protokolle; ein längeres Warten ändert diesen Zustand nicht.
- **Catalogue health expected UP:** Mindestens ein Katalogbestandteil ist
  nicht betriebsbereit. Prüfen Sie die ausgegebenen Zustände und Protokolle
  von Katalog, Fuseki, PostgreSQL und Identitätsdienst.
- **Functional verification failed:** Der Katalog ist erreichbar, kann aber
  die semantische Prüfoperation nicht erfolgreich ausführen. Beheben Sie den
  gemeldeten Katalogfehler; es gibt absichtlich keinen automatischen
  Aufwärm- oder Wiederholungsversuch.
- **`/readyz` meldet 503:** Die Initialisierung ist noch nicht vollständig.
  Suchen Sie in der Bereitstellungsausgabe nach dem ersten konkreten Fehler,
  statt die Wartezeit zu verlängern.

Eine technische Konfigurations- und Diagnosebeschreibung enthält der
[Betriebsleitfaden](../fc-integration/fc-integration-guide.md).
