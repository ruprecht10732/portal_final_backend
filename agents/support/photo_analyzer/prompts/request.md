Analyseer de {{ .PhotoCount }} foto('s) voor deze thuisdienst aanvraag.

Lead ID: {{ .LeadID }}
Service ID: {{ .ServiceID }}
{{- if .PreprocessingSection }}

## PREPROCESSING CONTEXT
{{ .PreprocessingSection }}
{{- end }}
{{- if .ServiceTypeSection }}

{{ .ServiceTypeSection }}
{{- end }}
{{- if .IntakeRequirementsSection }}

{{ .IntakeRequirementsSection }}
{{- end }}
{{- if .ContextInfoSection }}

{{ .ContextInfoSection }}
{{- end }}

## Analyseer elke foto zorgvuldig en voer uit:

### 1. VISUELE OBSERVATIES
- Welk specifiek probleem of situatie wordt getoond
- De geschatte omvang en complexiteit van het benodigde werk
- Factoren die prijs of tijdlijn kunnen beïnvloeden
- Veiligheidszorgen die aangepakt moeten worden

### 2. METINGEN (CRUCIAAL)
Gebruik foto's NIET als betrouwbare bron voor absolute meters, vierkante meters of volumes wanneer die niet expliciet zichtbaar of gelabeld zijn:
- Identificeer standaard componenten of configuraties, bijvoorbeeld enkel deurblad, dubbel glas, radiatorpaneel, groepenkast met meerdere groepen.
- Tel alleen aantallen die visueel ondubbelzinnig zichtbaar zijn.
- Leg alleen metingen vast als de waarde direct zichtbaar is op het product, op verpakking, via OCR, of anders expliciet in beeld staat.
- Gebruik Calculator alleen voor afgeleide berekeningen op basis van expliciet zichtbare of gelabelde waarden, niet op basis van gegokte referentie-objecten.
- Noteer elke meting met type (dimension/area/count/volume), waarde, eenheid en confidence.
- ANTIFOUT-REGEL: Het is beter om FlagOnsiteMeasurement aan te roepen dan een onjuiste meting te geven.
- Als exacte maatvoering nodig is voor prijsbepaling of je confidence niet "High" kan zijn (door hoek, perspectief, lensvervorming, onscherpte of ontbrekende schaal), roep FlagOnsiteMeasurement aan met de reden.
- Gebruik geen speculatieve referentie-objecten zoals deuren, stopcontacten of tegels om absolute afmetingen af te leiden.

### 3. TEKST EXTRACTIE (OCR)
Lees alle zichtbare tekst op foto's:
- Gebruik eventuele OCR assist candidates uit preprocessing als machine-read startpunt en verifieer ze tegen het beeld.
- Merknamen, modelnummers, serienummers
- Energielabels, typeplaten, CE-markeringen
- Afmetingen op verpakkingen of producten
- Waarschuwingsteksten

### 4. FEITCONTROLE (DISCREPANCIES)
Als er context of claims van de consument zijn meegegeven:
- Vergelijk elke claim met visuele bewijzen
- Noteer tegenstrijdigheden (bijv. "consument meldt lekkage maar geen vochtsporen zichtbaar")
- Dit helpt de Gatekeeper claims te valideren

### 5. PRODUCTZOEKTERMEN
Stel zoektermen voor die de Schatter kan gebruiken om materialen te vinden:
- Specifieke productnamen, materiaalsoorten
- Nederlandse en Engelse termen
- Merken en modellen als zichtbaar