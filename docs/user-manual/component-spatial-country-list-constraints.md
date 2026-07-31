# Länderlisten für räumliche Bedingungen

Mit einer räumlichen Bedingung legen Sie in einer wiederverwendbaren Vertragskomponente fest, für welche Länder eine Regel gilt. Die verfügbaren Länder und ihre dreistelligen Ländercodes stammen aus dem hinterlegten Fachkatalog.

## Voraussetzung

Sie sind als **Template Creator** angemeldet.

## Räumliche Bedingung anlegen

1. Öffnen Sie **Templates** und wählen Sie **New Template**.
2. Wählen Sie den Typ **Component**.
3. Öffnen Sie den Tab **Clauses**.
4. Gehen Sie in der gewünschten Klausel zu **Machine-readable meaning (ODRL)** und wählen Sie **+ constraint**.
5. Wählen Sie als Eigenschaft **access region (spatial)**.
6. Wählen Sie den passenden Vergleich:
   - **must equal** erlaubt genau ein Land.
   - **must be one of**, **must not be one of** und **must be all of** erlauben mehrere Länder.
7. Wählen Sie ein oder mehrere Länder aus. Bei einer Mehrfachauswahl verwenden Sie je nach Betriebssystem `Strg` oder `Cmd`, um mehrere Einträge zu markieren.

Die Auswahl zeigt die Ländernamen zusammen mit ihren Codes, beispielsweise **Germany (DEU)**. Verfügbar sind derzeit Germany (DEU), Austria (AUT), Switzerland (CHE) und United States (USA).

## Sichtbare Hinweise

- Solange bei **must equal** kein Land gewählt wurde, zeigt die Auswahl **choose value**.
- Wenn Sie von einer Mehrfachbedingung zu **must equal** wechseln, wird das Eingabefeld als Einzelauswahl dargestellt. Prüfen Sie anschließend, welches Land ausgewählt ist.
- Die Länder werden aus dem Fachkatalog bereitgestellt. Fehlt ein erwartetes Land, kann es in diesem Dialog nicht als freier Text ergänzt werden.
