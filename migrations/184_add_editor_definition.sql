-- +goose Up
-- +goose StatementBegin

-- Add editor_definition column for V2 admin flow builder format.
-- The existing 'definition' column keeps the V1 SDUI format for the offerte-wizard intake.
-- The new 'editor_definition' column stores the V2 form-builder format for the admin editor.
ALTER TABLE rac_product_flows
    ADD COLUMN editor_definition JSONB;

COMMENT ON COLUMN rac_product_flows.editor_definition IS 'V2 FlowDefinition JSON for the admin flow builder editor. NULL = not yet converted.';

-- ─── Convert houten-kozijnen ─────────────────────────────────────────────────
UPDATE rac_product_flows
SET editor_definition = '{
  "version": 2,
  "settings": {
    "productGroup": "houten-kozijnen",
    "summaryTitle": "Kozijnopname samenvatting",
    "summaryDescription": "Overzicht van de houten kozijnconfiguratie"
  },
  "steps": [
    {
      "id": "subtype",
      "title": "Welk houten kozijn gaan we inmeten?",
      "description": "Kies eerst het juiste kozijnsubtype.",
      "visibleWhen": null,
      "inputs": [
        {
          "id": "input-subtype",
          "type": "image-select",
          "label": "Kozijntype",
          "required": true,
          "visibleWhen": null,
          "draftField": "subtypeId",
          "options": [
            { "id": "fixed-glass", "label": "Houten vast glas kozijn", "description": "Vaste glaselementen en vaste ramen zonder draaiende delen.", "imagePath": "/intake/kozijn-subtypes/fixed-glass.svg", "available": true },
            { "id": "tilt-turn", "label": "Draaikiepramen", "description": "Kiep- en draaibare houten ramen.", "imagePath": "/intake/kozijn-subtypes/tilt-turn.svg", "available": false },
            { "id": "casement", "label": "Houten draairamen", "description": "Draaiende houten ramen.", "imagePath": "/intake/kozijn-subtypes/casement.svg", "available": false },
            { "id": "top-hung", "label": "Houten valramen", "description": "Valramen en bovenlichten in hout.", "imagePath": "/intake/kozijn-subtypes/top-hung.svg", "available": false },
            { "id": "door-frames", "label": "Houten deurkozijnen", "description": "Houten deurkozijnen en deurcombinaties.", "imagePath": "/intake/kozijn-subtypes/door-frames.svg", "available": false },
            { "id": "loose-windows", "label": "Houten losse ramen", "description": "Losse ramen binnen een houten kozijnoplossing.", "imagePath": "/intake/kozijn-subtypes/loose-windows.svg", "available": false },
            { "id": "awning", "label": "Houten uitzetramen", "description": "Uitzetramen en naar buiten uitzetbare delen.", "imagePath": "/intake/kozijn-subtypes/awning.svg", "available": false }
          ]
        }
      ],
      "goToRules": [],
      "preset": null
    },
    {
      "id": "preset",
      "title": "Kies een preset configuratie",
      "description": "Selecteer de basisindeling van het kozijn.",
      "visibleWhen": null,
      "inputs": [
        {
          "id": "input-preset",
          "type": "image-select",
          "label": "Preset configuratie",
          "required": true,
          "visibleWhen": null,
          "draftField": "presetId",
          "options": [
            { "id": "vast-glas-1-hout", "label": "Vast glas | 1 vak | hout", "description": "Enkel vast glaskozijn in hout.", "imagePath": "/intake/fixed-glass-presets/vast-glas-1-hout.svg" },
            { "id": "vast-glas-2-mahonie", "label": "Vast glas | 2 vakken | mahonie", "description": "Twee vakken in mahonie.", "imagePath": "/intake/fixed-glass-presets/vast-glas-2-mahonie.svg" },
            { "id": "vast-glas-2-horizontaal-hout", "label": "Vast glas | 2 vakken horizontaal | hout", "description": "Horizontale verdeling in twee vakken.", "imagePath": "/intake/fixed-glass-presets/vast-glas-2-horizontaal-hout.svg" },
            { "id": "vast-glas-3-mahonie", "label": "Vast glas | 3 vakken | mahonie", "description": "Drie vakken in mahonie.", "imagePath": "/intake/fixed-glass-presets/vast-glas-3-mahonie.svg" },
            { "id": "vast-glas-3-horizontaal-hout", "label": "Vast glas | 3 vakken horizontaal | hout", "description": "Horizontale verdeling in drie vakken.", "imagePath": "/intake/fixed-glass-presets/vast-glas-3-horizontaal-hout.svg" },
            { "id": "vast-glas-2-verticaal-mahonie", "label": "Vast glas | 2 vakken verticaal | mahonie", "description": "Verticale verdeling in twee vakken.", "imagePath": "/intake/fixed-glass-presets/vast-glas-2-verticaal-mahonie.svg" },
            { "id": "vast-glas-2-verticaal-hout", "label": "Vast glas | 2 vakken verticaal | hout", "description": "Verticale verdeling in twee vakken.", "imagePath": "/intake/fixed-glass-presets/vast-glas-2-verticaal-hout.svg" },
            { "id": "vast-glas-3-verticaal-mahonie", "label": "Vast glas | 3 vakken verticaal | mahonie", "description": "Verticale verdeling in drie vakken.", "imagePath": "/intake/fixed-glass-presets/vast-glas-3-verticaal-mahonie.svg" },
            { "id": "vast-glas-3-verticaal-hout", "label": "Vast glas | 3 vakken verticaal | hout", "description": "Verticale verdeling in drie vakken.", "imagePath": "/intake/fixed-glass-presets/vast-glas-3-verticaal-hout.svg" },
            { "id": "vast-raam-buitenzijde-mahonie", "label": "Vast raam (buitenzijde) | 1 vak | mahonie", "description": "Vast raam aan buitenzijde in mahonie.", "imagePath": "/intake/fixed-glass-presets/vast-raam-buitenzijde-mahonie.svg" },
            { "id": "vast-raam-buitenzijde-hout", "label": "Vast raam (buitenzijde) | 1 vak | hout", "description": "Vast raam aan buitenzijde in hout.", "imagePath": "/intake/fixed-glass-presets/vast-raam-buitenzijde-hout.svg" },
            { "id": "vast-raam-binnenzijde-mahonie", "label": "Vast raam (binnenzijde) | 1 vak | mahonie", "description": "Vast raam aan binnenzijde in mahonie.", "imagePath": "/intake/fixed-glass-presets/vast-raam-binnenzijde-mahonie.svg" },
            { "id": "vast-raam-binnenzijde-hout", "label": "Vast raam (binnenzijde) | 1 vak | hout", "description": "Vast raam aan binnenzijde in hout.", "imagePath": "/intake/fixed-glass-presets/vast-raam-binnenzijde-hout.svg" }
          ]
        }
      ],
      "goToRules": [],
      "preset": null
    },
    {
      "id": "width",
      "title": "Breedte buitenmaten",
      "description": "Meet de breedte op drie punten: boven, midden en onder.",
      "visibleWhen": null,
      "inputs": [
        { "id": "input-width-top", "type": "number", "label": "Breedte boven", "required": true, "visibleWhen": null, "draftField": "widthTop", "unit": "mm", "min": 100, "max": 5000 },
        { "id": "input-width-middle", "type": "number", "label": "Breedte midden", "required": true, "visibleWhen": null, "draftField": "widthMiddle", "unit": "mm", "min": 100, "max": 5000 },
        { "id": "input-width-bottom", "type": "number", "label": "Breedte onder", "required": true, "visibleWhen": null, "draftField": "widthBottom", "unit": "mm", "min": 100, "max": 5000 }
      ],
      "goToRules": [],
      "preset": null
    },
    {
      "id": "height",
      "title": "Hoogte buitenmaten",
      "description": "Meet de hoogte op drie punten: links, midden en rechts.",
      "visibleWhen": null,
      "inputs": [
        { "id": "input-height-left", "type": "number", "label": "Hoogte links", "required": true, "visibleWhen": null, "draftField": "heightLeft", "unit": "mm", "min": 100, "max": 5000 },
        { "id": "input-height-center", "type": "number", "label": "Hoogte midden", "required": true, "visibleWhen": null, "draftField": "heightCenter", "unit": "mm", "min": 100, "max": 5000 },
        { "id": "input-height-right", "type": "number", "label": "Hoogte rechts", "required": true, "visibleWhen": null, "draftField": "heightRight", "unit": "mm", "min": 100, "max": 5000 }
      ],
      "goToRules": [],
      "preset": null
    },
    {
      "id": "specs",
      "title": "Specificaties",
      "description": "Voer de profieldiepte en overige specificaties in.",
      "visibleWhen": null,
      "inputs": [
        { "id": "input-profile", "type": "number", "label": "Profieldiepte", "required": true, "visibleWhen": null, "draftField": "profile", "unit": "mm", "min": 50, "max": 200 },
        { "id": "input-onderdorpel", "type": "number", "label": "Onderdorpel", "required": false, "visibleWhen": null, "draftField": "onderdorpel", "unit": "mm", "min": 0, "max": 200 },
        { "id": "input-ventilatierooster", "type": "number", "label": "Ventilatierooster", "required": false, "visibleWhen": null, "draftField": "ventilatierooster", "unit": "mm", "min": 0, "max": 500 },
        {
          "id": "input-glazing",
          "type": "radio",
          "label": "Beglazing",
          "required": true,
          "visibleWhen": null,
          "draftField": "glazing",
          "options": [
            { "id": "hr-plus-plus", "label": "HR++ Glas", "description": "Standaard isolerend dubbel glas" },
            { "id": "triple", "label": "Triple Glas", "description": "Extra isolatie voor hoge prestaties" },
            { "id": "none", "label": "Geen glas", "description": "Alleen het frame, zonder beglazing" }
          ]
        }
      ],
      "goToRules": [],
      "preset": null
    },
    {
      "id": "media",
      "title": "Leg de situatie vast",
      "description": "Maak foto''s van het huidige kozijn, de bereikbaarheid voor montage en eventuele obstakels.",
      "visibleWhen": null,
      "inputs": [
        { "id": "input-media", "type": "file-upload", "label": "Foto''s", "required": false, "visibleWhen": null, "draftField": "mediaFiles", "accept": "image/*", "multiple": true }
      ],
      "goToRules": [],
      "preset": null
    },
    {
      "id": "review",
      "title": "Controleer de kozijnopname",
      "description": "Bekijk de samenvatting van maten en specificaties voordat je deze opname opslaat.",
      "visibleWhen": null,
      "inputs": [],
      "goToRules": [],
      "preset": null
    }
  ]
}'::jsonb
WHERE product_group_id = 'houten-kozijnen';

-- ─── Convert deuren ──────────────────────────────────────────────────────────
UPDATE rac_product_flows
SET editor_definition = '{
  "version": 2,
  "settings": {
    "productGroup": "deuren",
    "summaryTitle": "Deuropname samenvatting",
    "summaryDescription": "Overzicht van de deurconfiguratie"
  },
  "steps": [
    {
      "id": "category",
      "title": "Wat voor deuren wil je laten opnemen?",
      "description": "Kies de juiste deurcategorie zodat we de vervolgintake hierop kunnen aansluiten.",
      "visibleWhen": null,
      "inputs": [
        {
          "id": "input-category",
          "type": "image-select",
          "label": "Deurcategorie",
          "required": true,
          "visibleWhen": null,
          "draftField": "categoryId",
          "options": [
            { "id": "binnendeuren", "label": "Binnendeuren", "description": "Stompe en opdekdeuren voor binnenruimtes en doorgangen.", "imagePath": "/intake/door-categories/binnendeuren.svg", "available": true },
            { "id": "buitendeuren", "label": "Buitendeuren", "description": "Voordeuren, achterdeuren en andere buitenschil-oplossingen.", "imagePath": "/intake/door-categories/buitendeuren.svg", "available": true },
            { "id": "tuindeuren", "label": "Tuindeuren", "description": "Dubbele terras- en tuindeuren met doorgang naar buiten.", "imagePath": "/intake/door-categories/tuindeuren.svg", "available": true },
            { "id": "tuinpoorten", "label": "Tuinpoorten", "description": "Poorten en afsluitingen voor tuin, oprit of erfgrens.", "imagePath": "/intake/door-categories/tuinpoorten.svg", "available": true }
          ]
        }
      ],
      "goToRules": [],
      "preset": null
    },
    {
      "id": "frame",
      "title": "Wordt het kozijn meegenomen?",
      "description": "Geef aan of het kozijn mee wordt opgenomen of dat alleen het deurblad relevant is.",
      "visibleWhen": { "op": "truthy", "field": "categoryId" },
      "inputs": [
        {
          "id": "input-frame-option",
          "type": "image-select",
          "label": "Kozijnoptie",
          "required": true,
          "visibleWhen": null,
          "draftField": "frameOptionId",
          "options": [
            { "id": "inclusief-kozijn", "label": "Inclusief kozijn", "description": "Deur en kozijn worden samen opgenomen als complete combinatie.", "imagePath": "/intake/door-frame-options/inclusief-kozijn.svg", "available": true },
            { "id": "exclusief-kozijn", "label": "Exclusief kozijn", "description": "Alleen de deur of het deurblad wordt opgenomen, zonder kozijn.", "imagePath": "/intake/door-frame-options/exclusief-kozijn.svg", "available": true }
          ]
        }
      ],
      "goToRules": [],
      "preset": null
    },
    {
      "id": "door-type",
      "title": "Wat voor type deur is het?",
      "description": "Kies het juiste deurtype.",
      "visibleWhen": { "op": "and", "conditions": [{ "op": "in", "field": "categoryId", "values": ["binnendeuren", "buitendeuren"] }, { "op": "truthy", "field": "frameOptionId" }] },
      "inputs": [
        {
          "id": "input-door-type",
          "type": "image-select",
          "label": "Deurtype",
          "required": true,
          "visibleWhen": null,
          "draftField": "interiorDoorTypeId",
          "options": [
            { "id": "opdekdeuren", "label": "Opdekdeuren", "description": "Binnendeuren die deels op het kozijn vallen met scharnieren in opdeksysteem.", "imagePath": "/intake/interior-door-types/opdekdeuren.svg", "available": true },
            { "id": "stompe-deuren", "label": "Stompe deuren", "description": "Recht afgewerkte binnendeuren die volledig in het kozijn vallen.", "imagePath": "/intake/interior-door-types/stompe-deuren.svg", "available": true }
          ]
        }
      ],
      "goToRules": [],
      "preset": null
    },
    {
      "id": "supplier",
      "title": "Wie levert de deur?",
      "description": "Geef aan wie de deur levert.",
      "visibleWhen": { "op": "and", "conditions": [{ "op": "in", "field": "categoryId", "values": ["binnendeuren", "buitendeuren"] }, { "op": "truthy", "field": "frameOptionId" }, { "op": "truthy", "field": "interiorDoorTypeId" }] },
      "inputs": [
        {
          "id": "input-supplier",
          "type": "image-select",
          "label": "Leverancier",
          "required": true,
          "visibleWhen": null,
          "draftField": "supplierOptionId",
          "options": [
            { "id": "lead-levert-deur", "label": "Lead levert de deur", "description": "De klant verzorgt zelf de deur; wij nemen alleen maatvoering en montagecontext op.", "imagePath": "/intake/door-supplier-options/lead-levert-deur.svg", "available": true },
            { "id": "wij-leveren-deur", "label": "Wij leveren de deur", "description": "Wij leveren de deur mee als onderdeel van de offerte en opname.", "imagePath": "/intake/door-supplier-options/wij-leveren-deur.svg", "available": true }
          ]
        }
      ],
      "goToRules": [],
      "preset": null
    },
    {
      "id": "measurements",
      "title": "Maten opmeten",
      "description": "Meet de deur- of kozijnmaten op.",
      "visibleWhen": { "op": "and", "conditions": [{ "op": "in", "field": "categoryId", "values": ["binnendeuren", "buitendeuren"] }, { "op": "truthy", "field": "supplierOptionId" }] },
      "inputs": [
        { "id": "input-width", "type": "number", "label": "Breedte", "required": true, "visibleWhen": null, "draftField": "widthMm", "unit": "mm", "min": 100, "max": 3000 },
        { "id": "input-height", "type": "number", "label": "Hoogte", "required": true, "visibleWhen": null, "draftField": "heightMm", "unit": "mm", "min": 100, "max": 3500 },
        { "id": "input-thickness", "type": "number", "label": "Dikte", "required": false, "visibleWhen": null, "draftField": "thicknessMm", "unit": "mm", "min": 10, "max": 200 }
      ],
      "goToRules": [],
      "preset": null
    },
    {
      "id": "preferences",
      "title": "Klantwensen",
      "description": "Leg de klantwensen en voorbeeldvoorkeuren vast.",
      "visibleWhen": { "op": "and", "conditions": [{ "op": "in", "field": "categoryId", "values": ["binnendeuren", "buitendeuren"] }, { "op": "truthy", "field": "widthMm" }, { "op": "truthy", "field": "heightMm" }, { "op": "neq", "field": "supplierOptionId", "value": "wij-leveren-deur" }] },
      "inputs": [
        {
          "id": "input-example-preference",
          "type": "image-select",
          "label": "Voorbeelden beschikbaar?",
          "required": false,
          "visibleWhen": null,
          "draftField": "examplePreferenceId",
          "options": [
            { "id": "examples-available", "label": "Ja, voorbeelden beschikbaar", "description": "De klant kan voorbeelden, foto''s of links delen.", "imagePath": "/intake/door-preference-options/examples-available.svg", "available": true },
            { "id": "no-examples-yet", "label": "Nog geen voorbeelden", "description": "De klant heeft nog geen concrete voorbeelden.", "imagePath": "/intake/door-preference-options/no-examples-yet.svg", "available": true }
          ]
        },
        { "id": "input-customer-wishes", "type": "textarea", "label": "Klantwensen", "description": "Beschrijf de stijlwensen van de klant", "required": false, "visibleWhen": null, "draftField": "customerWishes", "rows": 4 },
        { "id": "input-example-notes", "type": "textarea", "label": "Voorbeeld notities", "description": "Links, referenties of aanvullende opmerkingen", "required": false, "visibleWhen": null, "draftField": "exampleNotes", "rows": 3 }
      ],
      "goToRules": [],
      "preset": null
    },
    {
      "id": "media",
      "title": "Leg de situatie vast",
      "description": "Maak foto''s van de huidige deur of het kozijn, de bereikbaarheid voor montage en eventuele obstakels.",
      "visibleWhen": { "op": "truthy", "field": "frameOptionId" },
      "inputs": [
        { "id": "input-media", "type": "file-upload", "label": "Foto''s", "required": false, "visibleWhen": null, "draftField": "mediaFiles", "accept": "image/*", "multiple": true }
      ],
      "goToRules": [],
      "preset": null
    },
    {
      "id": "review",
      "title": "Controleer de deuropname",
      "description": "Bekijk de gekozen uitvoering en maten voordat je deze opname opslaat.",
      "visibleWhen": null,
      "inputs": [],
      "goToRules": [],
      "preset": null
    }
  ]
}'::jsonb
WHERE product_group_id = 'deuren';

-- ─── Convert tuinpoorten ─────────────────────────────────────────────────────
UPDATE rac_product_flows
SET editor_definition = '{
  "version": 2,
  "settings": {
    "productGroup": "tuinpoorten",
    "summaryTitle": "Poortopname samenvatting",
    "summaryDescription": "Overzicht van de tuinpoortconfiguratie"
  },
  "steps": [
    {
      "id": "frame",
      "title": "Wordt het kozijn meegenomen?",
      "description": "Geef aan of het kozijn mee wordt opgenomen.",
      "visibleWhen": null,
      "inputs": [
        {
          "id": "input-frame-option",
          "type": "image-select",
          "label": "Kozijnoptie",
          "required": true,
          "visibleWhen": null,
          "draftField": "frameOptionId",
          "options": [
            { "id": "inclusief-kozijn", "label": "Inclusief kozijn", "description": "Deur en kozijn worden samen opgenomen als complete combinatie.", "imagePath": "/intake/door-frame-options/inclusief-kozijn.svg", "available": true },
            { "id": "exclusief-kozijn", "label": "Exclusief kozijn", "description": "Alleen de deur of het deurblad wordt opgenomen, zonder kozijn.", "imagePath": "/intake/door-frame-options/exclusief-kozijn.svg", "available": true }
          ]
        }
      ],
      "goToRules": [],
      "preset": null
    },
    {
      "id": "media",
      "title": "Leg de situatie vast",
      "description": "Maak foto''s van de huidige poort, doorgang, bevestigingspunten en eventuele obstakels rond montage of afwerking.",
      "visibleWhen": { "op": "truthy", "field": "frameOptionId" },
      "inputs": [
        { "id": "input-media", "type": "file-upload", "label": "Foto''s", "required": false, "visibleWhen": null, "draftField": "mediaFiles", "accept": "image/*", "multiple": true }
      ],
      "goToRules": [],
      "preset": null
    },
    {
      "id": "review",
      "title": "Controleer de poortopname",
      "description": "Bekijk de gekozen uitvoering voordat je deze opname opslaat.",
      "visibleWhen": { "op": "truthy", "field": "frameOptionId" },
      "inputs": [],
      "goToRules": [],
      "preset": null
    }
  ]
}'::jsonb
WHERE product_group_id = 'tuinpoorten';

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE rac_product_flows DROP COLUMN IF EXISTS editor_definition;
-- +goose StatementEnd
