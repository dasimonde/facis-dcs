# Rollen für zweiseitige Vertragsvorlagen festlegen

## Nutzen

Eine zweiseitige Vertragsvorlage legt die beiden Vertragsrollen bereits in
ihren maschinenlesbaren Regeln fest. Bei der Vertragserstellung wählen Sie
anschließend aus diesen beiden Rollen aus, welche Rolle Ihre Organisation
übernimmt. Dadurch bleiben die Rollenbezeichnungen in Vorlage und Vertrag
einheitlich.

## Voraussetzungen und Rollen

- Als **Template Creator** können Sie die Rollen einer Vertragsvorlage in den
  maschinenlesbaren Regeln festlegen.
- Als **Contract Creator** können Sie aus einer freigegebenen Vorlage einen
  Vertrag erstellen.
- Der zentrale Fachkatalog muss erreichbar sein, damit die verfügbaren Rollen
  geladen werden können.

## Vorlage vorbereiten

1. Öffnen Sie die gewünschte Vertragsvorlage zur Bearbeitung und wechseln Sie
   zu den Klauseln.
2. Öffnen oder ergänzen Sie bei einer Klausel die maschinenlesbare Regel.
3. Wählen Sie unter **Granted by (assigner)** und **Applies to (assignee)** die
   passenden Rollen aus. In diesen beiden Feldern stehen ausschließlich Rollen
   aus dem Fachkatalog zur Auswahl; eigene Texteingaben sind nicht möglich.
   Der Katalog enthält genau diese vier Rollen:

   | Anzeige | Gespeicherter Rollenbegriff |
   |---|---|
   | provider | `https://w3id.org/facis/dcs/taxonomy/v1#role-provider` |
   | customer | `https://w3id.org/facis/dcs/taxonomy/v1#role-customer` |
   | supplier | `https://w3id.org/facis/dcs/taxonomy/v1#role-supplier` |
   | client | `https://w3id.org/facis/dcs/taxonomy/v1#role-client` |

   Die Oberfläche zeigt die verständliche Bezeichnung, speichert aber immer
   den vollständigen Rollenbegriff.
4. Wiederholen Sie die Auswahl für alle direkten Regeln der Vorlage. Welche
   Rolle erteilt und welche Rolle empfängt, darf sich zwischen Regeln ändern.
5. Stellen Sie sicher, dass über alle direkten Regeln hinweg genau zwei
   verschiedene Rollen verwendet werden. Wiederholte Verwendungen derselben
   Rolle zählen nicht als zusätzliche Rolle.
6. Speichern und durchlaufen Sie den vorgesehenen Freigabeprozess der Vorlage.

## Vertrag erstellen

1. Öffnen Sie die Vertragserstellung und wählen Sie eine freigegebene Vorlage.
2. Wählen Sie **Create**.
3. Warten Sie, bis die Rollen der Vorlage geladen sind.
4. Wählen Sie unter **Your role in this contract** eine der beiden angebotenen
   Rollen. Die Auswahl zeigt die Bezeichnung, übermittelt aber den vollständigen
   Rollenbegriff. Es ist absichtlich keine Rolle vorausgewählt und keine freie
   Eingabe möglich.
5. Ergänzen Sie bei Bedarf die Gegenstelle und die leseberechtigten
   Organisationen.
6. Wählen Sie **Apply**. Ihre Organisation wird der gewählten Vertragsrolle
   zugeordnet; die andere Rolle bleibt für die Gegenstelle offen.

## Sichtbare Fehlerfälle

- **Loading template roles…:** Die Rollen werden noch geladen. Die Auswahl und
  **Apply** bleiben bis zum Abschluss gesperrt.
- **Template roles could not be loaded.:** Die Vorlage oder ihr Rollenkatalog
  konnte nicht geladen werden. Laden Sie die Vorlage erneut; eine freie
  Ersatzeingabe ist nicht möglich.
- **Contract creation requires exactly two catalogued roles in the selected
  template.:** Die Vorlage verwendet nicht genau zwei verschiedene
  katalogisierte Rollen. Korrigieren und freigeben Sie die Vorlage, bevor Sie
  daraus einen Vertrag erstellen.
- **Apply ist deaktiviert:** Wählen Sie zuerst unter **Your role in this
  contract** eine Rolle aus.
- **Vertragserstellung wird nach der Auswahl abgelehnt:** Die gewählte Rolle
  gehört nicht mehr zu den genau zwei Rollen der geladenen Vorlage. Laden Sie
  die Vorlage neu und wählen Sie erneut; die Rolle kann nicht als freier Text
  erzwungen werden.
- **Eine Vorlage oder Integration verwendet nur `provider`, `customer`,
  `supplier` oder `client`:** Diese Kurzangaben sind keine gültigen
  Rollenbegriffe. Verwenden Sie den jeweils vollständigen Rollenbegriff aus der
  Tabelle; die Vertragserstellung weist Kurzangaben zurück.
