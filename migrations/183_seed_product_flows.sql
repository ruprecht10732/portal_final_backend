-- +goose Up
-- +goose StatementBegin
-- Seed global default flow definitions for all 3 product groups.
-- organization_id = NULL means these are the shared defaults.

-- ─── Deuren (8 steps) ────────────────────────────────────────────────────────
INSERT INTO rac_product_flows (organization_id, product_group_id, definition) VALUES (
  NULL,
  'deuren',
  '{
    "steps": [
      {
        "id": "category",
        "type": "single-select-grid",
        "title": "Wat voor deuren wil je laten opnemen?",
        "description": "Kies de juiste deurcategorie zodat we de vervolgintake hierop kunnen aansluiten.",
        "options": [
          { "id": "binnendeuren", "label": "Binnendeuren", "description": "Stompe en opdekdeuren voor binnenruimtes en doorgangen.", "imagePath": "/intake/door-categories/binnendeuren.svg", "available": true },
          { "id": "buitendeuren", "label": "Buitendeuren", "description": "Voordeuren, achterdeuren en andere buitenschil-oplossingen.", "imagePath": "/intake/door-categories/buitendeuren.svg", "available": true },
          { "id": "tuindeuren", "label": "Tuindeuren", "description": "Dubbele terras- en tuindeuren met doorgang naar buiten.", "imagePath": "/intake/door-categories/tuindeuren.svg", "available": true },
          { "id": "tuinpoorten", "label": "Tuinpoorten", "description": "Poorten en afsluitingen voor tuin, oprit of erfgrens.", "imagePath": "/intake/door-categories/tuinpoorten.svg", "available": true }
        ],
        "visibleWhen": null,
        "completeWhen": { "op": "truthy", "field": "categoryId" },
        "inputMap": {
          "stepNumber": { "$meta": "stepNumber" },
          "totalSteps": { "$meta": "totalSteps" },
          "title": { "$stepField": "title" },
          "description": { "$stepField": "description" },
          "subtypeOptions": { "$stepOptions": true },
          "selectedSubtypeId": { "$draft": "categoryId" }
        },
        "autoAdvance": true,
        "outputMap": {
          "subtypeSelected": {
            "patchDraft": { "categoryId": { "$value": true } },
            "resetFields": ["frameOptionId", "interiorDoorTypeId", "supplierOptionId", "widthMm", "heightMm", "thicknessMm", "customerWishes", "examplePreferenceId", "exampleNotes"]
          }
        }
      },
      {
        "id": "frame",
        "type": "single-select-cards",
        "title": "Wordt het kozijn meegenomen?",
        "description": "Geef aan of het kozijn mee wordt opgenomen of dat alleen het deurblad relevant is.",
        "options": [
          { "id": "inclusief-kozijn", "label": "Inclusief kozijn", "description": "Deur en kozijn worden samen opgenomen als complete combinatie.", "imagePath": "/intake/door-frame-options/inclusief-kozijn.svg", "available": true },
          { "id": "exclusief-kozijn", "label": "Exclusief kozijn", "description": "Alleen de deur of het deurblad wordt opgenomen, zonder kozijn.", "imagePath": "/intake/door-frame-options/exclusief-kozijn.svg", "available": true }
        ],
        "visibleWhen": { "op": "truthy", "field": "categoryId" },
        "completeWhen": { "op": "truthy", "field": "frameOptionId" },
        "inputMap": {
          "totalSteps": { "$meta": "totalSteps" },
          "categoryLabel": { "$draft": "categoryId" },
          "showFutureFlowIndicator": { "$literal": false },
          "frameOptions": { "$stepOptions": true },
          "selectedFrameOptionId": { "$draft": "frameOptionId" }
        },
        "autoAdvance": true,
        "outputMap": {
          "frameOptionSelected": {
            "patchDraft": { "frameOptionId": { "$value": true } },
            "resetFields": ["interiorDoorTypeId", "supplierOptionId", "widthMm", "heightMm", "thicknessMm", "customerWishes", "examplePreferenceId", "exampleNotes"]
          }
        }
      },
      {
        "id": "door-type",
        "type": "single-select-grid",
        "title": "Wat voor type deur is het?",
        "description": "Kies het juiste deurtype.",
        "options": [
          { "id": "opdekdeuren", "label": "Opdekdeuren", "description": "Binnendeuren die deels op het kozijn vallen met scharnieren in opdeksysteem.", "imagePath": "/intake/interior-door-types/opdekdeuren.svg", "available": true },
          { "id": "stompe-deuren", "label": "Stompe deuren", "description": "Recht afgewerkte binnendeuren die volledig ín het kozijn vallen.", "imagePath": "/intake/interior-door-types/stompe-deuren.svg", "available": true }
        ],
        "visibleWhen": {
          "op": "and",
          "conditions": [
            { "op": "in", "field": "categoryId", "values": ["binnendeuren", "buitendeuren"] },
            { "op": "truthy", "field": "frameOptionId" }
          ]
        },
        "completeWhen": { "op": "truthy", "field": "interiorDoorTypeId" },
        "inputMap": {
          "totalSteps": { "$meta": "totalSteps" },
          "categoryLabel": { "$draft": "categoryId" },
          "frameOptionLabel": { "$draft": "frameOptionId" },
          "interiorDoorTypeOptions": { "$stepOptions": true },
          "selectedInteriorDoorTypeId": { "$draft": "interiorDoorTypeId" }
        },
        "autoAdvance": true,
        "outputMap": {
          "interiorDoorTypeSelected": {
            "patchDraft": { "interiorDoorTypeId": { "$value": true } },
            "resetFields": ["supplierOptionId"]
          }
        }
      },
      {
        "id": "supplier",
        "type": "single-select-cards",
        "title": "Wie levert de deur?",
        "description": "Geef aan wie de deur levert.",
        "options": [
          { "id": "lead-levert-deur", "label": "Lead levert de deur", "description": "De klant of lead verzorgt zelf de deur; wij nemen alleen maatvoering en montagecontext op.", "imagePath": "/intake/door-supplier-options/lead-levert-deur.svg", "available": true },
          { "id": "wij-leveren-deur", "label": "Wij leveren de deur", "description": "Wij leveren de deur mee als onderdeel van de offerte en opname.", "imagePath": "/intake/door-supplier-options/wij-leveren-deur.svg", "available": true }
        ],
        "visibleWhen": {
          "op": "and",
          "conditions": [
            { "op": "in", "field": "categoryId", "values": ["binnendeuren", "buitendeuren"] },
            { "op": "truthy", "field": "frameOptionId" },
            { "op": "truthy", "field": "interiorDoorTypeId" }
          ]
        },
        "completeWhen": { "op": "truthy", "field": "supplierOptionId" },
        "inputMap": {
          "totalSteps": { "$meta": "totalSteps" },
          "interiorDoorTypeLabel": { "$draft": "interiorDoorTypeId" },
          "supplierOptions": { "$stepOptions": true },
          "selectedSupplierOptionId": { "$draft": "supplierOptionId" }
        },
        "autoAdvance": true,
        "outputMap": {
          "supplierOptionSelected": {
            "patchDraft": { "supplierOptionId": { "$value": true } }
          }
        }
      },
      {
        "id": "measurements",
        "type": "measurements-door",
        "title": "Maten opmeten",
        "description": "Meet de deur- of kozijnmaten op.",
        "visibleWhen": {
          "op": "and",
          "conditions": [
            { "op": "in", "field": "categoryId", "values": ["binnendeuren", "buitendeuren"] },
            { "op": "truthy", "field": "supplierOptionId" }
          ]
        },
        "completeWhen": {
          "op": "and",
          "conditions": [
            { "op": "truthy", "field": "widthMm" },
            { "op": "truthy", "field": "heightMm" }
          ]
        },
        "inputMap": {
          "totalSteps": { "$meta": "totalSteps" },
          "interiorDoorTypeLabel": { "$draft": "interiorDoorTypeId" },
          "frameOptionLabel": { "$draft": "frameOptionId" },
          "supplierOptionLabel": { "$draft": "supplierOptionId" },
          "measurementMode": { "$draft": "__measurementMode" },
          "initialValue": { "$draft": "__doorMeasurementValues" }
        },
        "outputMap": {
          "measurementsChanged": {
            "patchDraft": {
              "widthMm": { "$valueField": "widthMm" },
              "heightMm": { "$valueField": "heightMm" },
              "thicknessMm": { "$valueField": "thicknessMm" }
            }
          },
          "validityChanged": {}
        }
      },
      {
        "id": "preferences",
        "type": "preferences-door",
        "title": "Klantwensen",
        "description": "Leg de klantwensen en voorbeeldvoorkeuren vast.",
        "visibleWhen": {
          "op": "and",
          "conditions": [
            { "op": "in", "field": "categoryId", "values": ["binnendeuren", "buitendeuren"] },
            { "op": "truthy", "field": "widthMm" },
            { "op": "truthy", "field": "heightMm" },
            { "op": "neq", "field": "supplierOptionId", "value": "wij-leveren-deur" }
          ]
        },
        "completeWhen": { "op": "truthy", "field": "customerWishes" },
        "inputMap": {
          "totalSteps": { "$meta": "totalSteps" },
          "interiorDoorTypeLabel": { "$draft": "interiorDoorTypeId" },
          "initialValue": { "$draft": "__doorPreferenceValues" },
          "selectedExamplePreferenceId": { "$draft": "examplePreferenceId" },
          "examplePreferenceOptions": { "$literal": [
            { "id": "examples-available", "label": "Ja, voorbeelden beschikbaar", "description": "De klant kan voorbeelden, foto\u0027s of links delen van deuren die zij mooi vindt.", "imagePath": "/intake/door-preference-options/examples-available.svg", "available": true },
            { "id": "no-examples-yet", "label": "Nog geen voorbeelden", "description": "De klant heeft nog geen concrete voorbeelden, maar we leggen de stijlwensen wel alvast vast.", "imagePath": "/intake/door-preference-options/no-examples-yet.svg", "available": true }
          ]}
        },
        "outputMap": {
          "preferencesChanged": {
            "patchDraft": {
              "customerWishes": { "$valueField": "customerWishes" },
              "examplePreferenceId": { "$valueField": "examplePreferenceId" },
              "exampleNotes": { "$valueField": "exampleNotes" }
            }
          },
          "validityChanged": {}
        }
      },
      {
        "id": "media",
        "type": "media-upload",
        "title": "Leg de situatie vast",
        "description": "Maak foto\u0027s van de huidige deur of het kozijn, de bereikbaarheid voor montage en eventuele obstakels.",
        "visibleWhen": {
          "op": "and",
          "conditions": [
            { "op": "truthy", "field": "frameOptionId" },
            {
              "op": "or",
              "conditions": [
                { "op": "not_in", "field": "categoryId", "values": ["binnendeuren", "buitendeuren"] },
                {
                  "op": "and",
                  "conditions": [
                    { "op": "truthy", "field": "widthMm" },
                    { "op": "truthy", "field": "heightMm" }
                  ]
                }
              ]
            }
          ]
        },
        "completeWhen": { "op": "truthy", "field": "__always" },
        "inputMap": {
          "stepNumber": { "$meta": "stepNumber" },
          "totalSteps": { "$meta": "totalSteps" },
          "title": { "$stepField": "title" },
          "description": { "$stepField": "description" }
        },
        "outputMap": {
          "filesChanged": { "mediaFiles": true }
        }
      },
      {
        "id": "review",
        "type": "review-summary",
        "title": "Controleer de deuropname",
        "description": "Bekijk de gekozen uitvoering en maten voordat je deze opname bij de afspraak opslaat.",
        "visibleWhen": {
          "op": "and",
          "conditions": [
            { "op": "truthy", "field": "frameOptionId" },
            {
              "op": "or",
              "conditions": [
                { "op": "not_in", "field": "categoryId", "values": ["binnendeuren", "buitendeuren"] },
                {
                  "op": "and",
                  "conditions": [
                    { "op": "truthy", "field": "widthMm" },
                    { "op": "truthy", "field": "heightMm" }
                  ]
                }
              ]
            },
            {
              "op": "or",
              "conditions": [
                { "op": "eq", "field": "supplierOptionId", "value": "wij-leveren-deur" },
                { "op": "not_in", "field": "categoryId", "values": ["binnendeuren", "buitendeuren"] },
                { "op": "truthy", "field": "customerWishes" }
              ]
            }
          ]
        },
        "completeWhen": null,
        "inputMap": {
          "stepNumber": { "$meta": "stepNumber" },
          "totalSteps": { "$meta": "totalSteps" },
          "title": { "$stepField": "title" },
          "description": { "$stepField": "description" }
        },
        "outputMap": {
          "editRequested": {}
        }
      }
    ],
    "reviewTemplate": [
      {
        "title": "Selectie",
        "editStepId": "category",
        "items": [
          { "label": "Productgroep", "source": { "$literal": "Deuren" } },
          { "label": "Categorie", "source": { "$draft": "categoryId", "format": "option-label", "stepId": "category" } },
          { "label": "Kozijn", "source": { "$draft": "frameOptionId", "format": "option-label", "stepId": "frame" } },
          { "label": "Type", "source": { "$draft": "interiorDoorTypeId", "format": "option-label", "stepId": "door-type" } },
          { "label": "Levering", "source": { "$draft": "supplierOptionId", "format": "option-label", "stepId": "supplier" } }
        ]
      },
      {
        "title": "Maatvoering",
        "editStepId": "measurements",
        "visibleWhen": { "op": "in", "field": "categoryId", "values": ["binnendeuren", "buitendeuren"] },
        "items": [
          { "label": "Breedte", "source": { "$draft": "widthMm", "format": "mm" } },
          { "label": "Hoogte", "source": { "$draft": "heightMm", "format": "mm" } },
          { "label": "Dikte/Diepte", "source": { "$draft": "thicknessMm", "format": "mm" } }
        ]
      },
      {
        "title": "Klantwensen",
        "editStepId": "preferences",
        "visibleWhen": {
          "op": "and",
          "conditions": [
            { "op": "in", "field": "categoryId", "values": ["binnendeuren", "buitendeuren"] },
            { "op": "neq", "field": "supplierOptionId", "value": "wij-leveren-deur" }
          ]
        },
        "items": [
          { "label": "Wensen", "source": { "$draft": "customerWishes" } },
          { "label": "Voorbeelden", "source": { "$draft": "examplePreferenceId", "format": "option-label", "stepId": "preferences" } },
          { "label": "Voorbeeldnotities", "source": { "$draft": "exampleNotes" } }
        ]
      }
    ],
    "payloadSchema": {
      "productGroup": "deuren",
      "categoryField": "categoryId",
      "categoryLabelFallback": "Deuren",
      "frameOptionField": "frameOptionId",
      "frameOptionLabelFallback": "Niet gekozen",
      "productTypeField": "interiorDoorTypeId",
      "productTypeLabelFallback": "Niet gekozen",
      "supplierField": "supplierOptionId",
      "supplierLabelFallback": "Niet gekozen",
      "measurementVisibleWhen": { "op": "in", "field": "categoryId", "values": ["binnendeuren", "buitendeuren"] },
      "measurementFields": [
        { "key": "doorWidthMm", "label": "Deur breedte", "unit": "mm", "draftField": "widthMm", "conditionalKey": { "condition": { "op": "eq", "field": "frameOptionId", "value": "inclusief-kozijn" }, "alternateKey": "frameWidthMm" }, "conditionalLabel": { "condition": { "op": "eq", "field": "frameOptionId", "value": "inclusief-kozijn" }, "alternateLabel": "Kozijn breedte" } },
        { "key": "doorHeightMm", "label": "Deur hoogte", "unit": "mm", "draftField": "heightMm", "conditionalKey": { "condition": { "op": "eq", "field": "frameOptionId", "value": "inclusief-kozijn" }, "alternateKey": "frameHeightMm" }, "conditionalLabel": { "condition": { "op": "eq", "field": "frameOptionId", "value": "inclusief-kozijn" }, "alternateLabel": "Kozijn hoogte" } },
        { "key": "doorThicknessMm", "label": "Deur dikte", "unit": "mm", "draftField": "thicknessMm", "conditionalKey": { "condition": { "op": "eq", "field": "frameOptionId", "value": "inclusief-kozijn" }, "alternateKey": "frameDepthMm" }, "conditionalLabel": { "condition": { "op": "eq", "field": "frameOptionId", "value": "inclusief-kozijn" }, "alternateLabel": "Kozijn diepte" } }
      ],
      "preferencesSchema": {
        "customerWishesField": "customerWishes",
        "examplePreferenceField": "examplePreferenceId",
        "exampleNotesField": "exampleNotes"
      }
    }
  }'::jsonb
);

-- ─── Houten kozijnen (7 steps) ──────────────────────────────────────────────
INSERT INTO rac_product_flows (organization_id, product_group_id, definition) VALUES (
  NULL,
  'houten-kozijnen',
  '{
    "steps": [
      {
        "id": "subtype",
        "type": "single-select-grid",
        "title": "Welk houten kozijn gaan we inmeten?",
        "description": "Kies eerst het juiste kozijnsubtype. De configurator voor vast glas is nu uitgewerkt.",
        "options": [
          { "id": "fixed-glass", "label": "Houten vast glas kozijn", "description": "Vaste glaselementen en vaste ramen zonder draaiende delen.", "imagePath": "/intake/kozijn-subtypes/fixed-glass.svg", "available": true },
          { "id": "tilt-turn", "label": "Draaikiepramen", "description": "Kiep- en draaibare houten ramen.", "imagePath": "/intake/kozijn-subtypes/tilt-turn.svg", "available": false },
          { "id": "casement", "label": "Houten draairamen", "description": "Draaiende houten ramen.", "imagePath": "/intake/kozijn-subtypes/casement.svg", "available": false },
          { "id": "top-hung", "label": "Houten valramen", "description": "Valramen en bovenlichten in hout.", "imagePath": "/intake/kozijn-subtypes/top-hung.svg", "available": false },
          { "id": "door-frames", "label": "Houten deurkozijnen", "description": "Houten deurkozijnen en deurcombinaties.", "imagePath": "/intake/kozijn-subtypes/door-frames.svg", "available": false },
          { "id": "loose-windows", "label": "Houten losse ramen", "description": "Losse ramen binnen een houten kozijnoplossing.", "imagePath": "/intake/kozijn-subtypes/loose-windows.svg", "available": false },
          { "id": "awning", "label": "Houten uitzetramen", "description": "Uitzetramen en naar buiten uitzetbare delen.", "imagePath": "/intake/kozijn-subtypes/awning.svg", "available": false }
        ],
        "visibleWhen": null,
        "completeWhen": { "op": "truthy", "field": "subtypeId" },
        "inputMap": {
          "stepNumber": { "$meta": "stepNumber" },
          "totalSteps": { "$meta": "totalSteps" },
          "title": { "$stepField": "title" },
          "description": { "$stepField": "description" },
          "subtypeOptions": { "$stepOptions": true },
          "selectedSubtypeId": { "$draft": "subtypeId" }
        },
        "autoAdvance": true,
        "outputMap": {
          "subtypeSelected": {
            "patchDraft": { "subtypeId": { "$value": true } }
          }
        }
      },
      {
        "id": "preset",
        "type": "kozijn-preset",
        "title": "Kies een preset configuratie",
        "description": "Selecteer de basisindeling van het kozijn.",
        "options": [
          { "id": "vast-glas-1-hout", "label": "Vast glas | 1 vak | hout", "description": "Enkel vast glaskozijn in hout.", "imagePath": "/intake/fixed-glass-presets/vast-glas-1-hout.svg", "metadata": { "woodType": "hardwood", "mullions": 0, "transoms": 0 } },
          { "id": "vast-glas-2-mahonie", "label": "Vast glas | 2 vakken | mahonie", "description": "Twee vakken in mahonie.", "imagePath": "/intake/fixed-glass-presets/vast-glas-2-mahonie.svg", "metadata": { "woodType": "mahogany", "mullions": 1, "transoms": 0 } },
          { "id": "vast-glas-2-horizontaal-hout", "label": "Vast glas | 2 vakken horizontaal | hout", "description": "Horizontale verdeling in twee vakken.", "imagePath": "/intake/fixed-glass-presets/vast-glas-2-horizontaal-hout.svg", "metadata": { "woodType": "hardwood", "mullions": 0, "transoms": 1 } },
          { "id": "vast-glas-3-mahonie", "label": "Vast glas | 3 vakken | mahonie", "description": "Drie vakken in mahonie.", "imagePath": "/intake/fixed-glass-presets/vast-glas-3-mahonie.svg", "metadata": { "woodType": "mahogany", "mullions": 2, "transoms": 0 } },
          { "id": "vast-glas-3-horizontaal-hout", "label": "Vast glas | 3 vakken horizontaal | hout", "description": "Horizontale verdeling in drie vakken.", "imagePath": "/intake/fixed-glass-presets/vast-glas-3-horizontaal-hout.svg", "metadata": { "woodType": "hardwood", "mullions": 0, "transoms": 2 } },
          { "id": "vast-glas-2-verticaal-mahonie", "label": "Vast glas | 2 vakken verticaal | mahonie", "description": "Verticale verdeling in twee vakken.", "imagePath": "/intake/fixed-glass-presets/vast-glas-2-verticaal-mahonie.svg", "metadata": { "woodType": "mahogany", "mullions": 1, "transoms": 0 } },
          { "id": "vast-glas-2-verticaal-hout", "label": "Vast glas | 2 vakken verticaal | hout", "description": "Verticale verdeling in twee vakken.", "imagePath": "/intake/fixed-glass-presets/vast-glas-2-verticaal-hout.svg", "metadata": { "woodType": "hardwood", "mullions": 1, "transoms": 0 } },
          { "id": "vast-glas-3-verticaal-mahonie", "label": "Vast glas | 3 vakken verticaal | mahonie", "description": "Verticale verdeling in drie vakken.", "imagePath": "/intake/fixed-glass-presets/vast-glas-3-verticaal-mahonie.svg", "metadata": { "woodType": "mahogany", "mullions": 2, "transoms": 0 } },
          { "id": "vast-glas-3-verticaal-hout", "label": "Vast glas | 3 vakken verticaal | hout", "description": "Verticale verdeling in drie vakken.", "imagePath": "/intake/fixed-glass-presets/vast-glas-3-verticaal-hout.svg", "metadata": { "woodType": "hardwood", "mullions": 2, "transoms": 0 } },
          { "id": "vast-raam-buitenzijde-mahonie", "label": "Vast raam (buitenzijde) | 1 vak | mahonie", "description": "Vast raam aan buitenzijde in mahonie.", "imagePath": "/intake/fixed-glass-presets/vast-raam-buitenzijde-mahonie.svg", "metadata": { "woodType": "mahogany", "mullions": 0, "transoms": 0 } },
          { "id": "vast-raam-buitenzijde-hout", "label": "Vast raam (buitenzijde) | 1 vak | hout", "description": "Vast raam aan buitenzijde in hout.", "imagePath": "/intake/fixed-glass-presets/vast-raam-buitenzijde-hout.svg", "metadata": { "woodType": "hardwood", "mullions": 0, "transoms": 0 } },
          { "id": "vast-raam-binnenzijde-mahonie", "label": "Vast raam (binnenzijde) | 1 vak | mahonie", "description": "Vast raam aan binnenzijde in mahonie.", "imagePath": "/intake/fixed-glass-presets/vast-raam-binnenzijde-mahonie.svg", "metadata": { "woodType": "mahogany", "mullions": 0, "transoms": 0 } },
          { "id": "vast-raam-binnenzijde-hout", "label": "Vast raam (binnenzijde) | 1 vak | hout", "description": "Vast raam aan binnenzijde in hout.", "imagePath": "/intake/fixed-glass-presets/vast-raam-binnenzijde-hout.svg", "metadata": { "woodType": "hardwood", "mullions": 0, "transoms": 0 } }
        ],
        "visibleWhen": null,
        "completeWhen": { "op": "truthy", "field": "presetId" },
        "inputMap": {
          "totalSteps": { "$meta": "totalSteps" },
          "presetOptions": { "$stepOptions": true },
          "selectedPresetId": { "$draft": "presetId" }
        },
        "autoAdvance": true,
        "outputMap": {
          "presetSelected": {
            "patchDraft": { "presetId": { "$value": true } }
          }
        }
      },
      {
        "id": "width",
        "type": "kozijn-dimensions",
        "title": "Breedte buitenmaten",
        "description": "Meet de breedte op drie punten: boven, midden en onder.",
        "visibleWhen": null,
        "completeWhen": { "op": "and", "conditions": [{ "op": "truthy", "field": "widthTop" }, { "op": "truthy", "field": "widthMiddle" }, { "op": "truthy", "field": "widthBottom" }] },
        "inputMap": {
          "stepNumber": { "$meta": "stepNumber" },
          "totalSteps": { "$meta": "totalSteps" },
          "title": { "$stepField": "title" },
          "description": { "$stepField": "description" },
          "sectionLabel": { "$literal": "Breedte in mm" },
          "fields": { "$literal": [
            { "label": "Boven", "inputId": "measurement-width-top", "statusLabel": "Breedte boven" },
            { "label": "Midden", "inputId": "measurement-width-middle", "statusLabel": "Breedte midden" },
            { "label": "Onder", "inputId": "measurement-width-bottom", "statusLabel": "Breedte onder" }
          ]},
          "initialValue": { "$draft": "__widthStepValue" }
        },
        "outputMap": {
          "dimensionsChanged": {
            "patchDraft": {
              "widthTop": { "$valueField": "first" },
              "widthMiddle": { "$valueField": "second" },
              "widthBottom": { "$valueField": "third" }
            }
          },
          "validityChanged": {}
        }
      },
      {
        "id": "height",
        "type": "kozijn-dimensions",
        "title": "Hoogte buitenmaten",
        "description": "Meet de hoogte op drie punten: links, midden en rechts.",
        "visibleWhen": null,
        "completeWhen": { "op": "and", "conditions": [{ "op": "truthy", "field": "heightLeft" }, { "op": "truthy", "field": "heightCenter" }, { "op": "truthy", "field": "heightRight" }] },
        "inputMap": {
          "stepNumber": { "$meta": "stepNumber" },
          "totalSteps": { "$meta": "totalSteps" },
          "title": { "$stepField": "title" },
          "description": { "$stepField": "description" },
          "sectionLabel": { "$literal": "Hoogte in mm" },
          "fields": { "$literal": [
            { "label": "Links", "inputId": "measurement-height-left", "statusLabel": "Hoogte links" },
            { "label": "Midden", "inputId": "measurement-height-center", "statusLabel": "Hoogte midden" },
            { "label": "Rechts", "inputId": "measurement-height-right", "statusLabel": "Hoogte rechts" }
          ]},
          "initialValue": { "$draft": "__heightStepValue" }
        },
        "outputMap": {
          "dimensionsChanged": {
            "patchDraft": {
              "heightLeft": { "$valueField": "first" },
              "heightCenter": { "$valueField": "second" },
              "heightRight": { "$valueField": "third" }
            }
          },
          "validityChanged": {}
        }
      },
      {
        "id": "specs",
        "type": "kozijn-specs",
        "title": "Specificaties",
        "description": "Voer de profieldiepte en overige specificaties in.",
        "visibleWhen": null,
        "completeWhen": { "op": "truthy", "field": "profile" },
        "inputMap": {
          "totalSteps": { "$meta": "totalSteps" },
          "previewData": { "$draft": "__previewData" },
          "paneCount": { "$draft": "__paneCount" },
          "mullions": { "$draft": "mullions" },
          "transoms": { "$draft": "transoms" },
          "selectedPresetLabel": { "$draft": "__presetLabel" },
          "selectedWoodLabel": { "$draft": "__woodLabel" },
          "initialValue": { "$draft": "__kozijnSpecsValue" },
          "glazingOptions": { "$literal": [
            { "label": "HR++ Glas", "value": "hr-plus-plus", "description": "Standaard isolerend dubbel glas", "explanation": "HR++ is de gangbare keuze voor goede isolatie." },
            { "label": "Triple Glas", "value": "triple", "description": "Extra isolatie voor hoge prestaties", "explanation": "Triple glas bestaat uit drie glaslagen." },
            { "label": "Geen glas", "value": "none", "description": "Alleen het frame, zonder beglazing", "explanation": "Kies dit als alleen het kozijn wordt geleverd." }
          ]},
          "validationMessage": { "$draft": "__savedMessage" }
        },
        "outputMap": {
          "configurationChanged": {
            "patchDraft": {
              "profile": { "$valueField": "profile" },
              "onderdorpel": { "$valueField": "onderdorpel" },
              "ventilatierooster": { "$valueField": "ventilatierooster" },
              "glazing": { "$valueField": "glazing" }
            }
          },
          "validityChanged": {}
        }
      },
      {
        "id": "media",
        "type": "media-upload",
        "title": "Leg de situatie vast",
        "description": "Maak foto\u0027s van het huidige kozijn, de bereikbaarheid voor montage en eventuele obstakels.",
        "visibleWhen": null,
        "completeWhen": { "op": "truthy", "field": "__always" },
        "inputMap": {
          "stepNumber": { "$meta": "stepNumber" },
          "totalSteps": { "$meta": "totalSteps" },
          "title": { "$stepField": "title" },
          "description": { "$stepField": "description" }
        },
        "outputMap": {
          "filesChanged": { "mediaFiles": true }
        }
      },
      {
        "id": "review",
        "type": "review-summary",
        "title": "Controleer de kozijnopname",
        "description": "Bekijk de samenvatting van maten en specificaties voordat je deze opname bij de afspraak opslaat.",
        "visibleWhen": null,
        "completeWhen": null,
        "inputMap": {
          "stepNumber": { "$meta": "stepNumber" },
          "totalSteps": { "$meta": "totalSteps" },
          "title": { "$stepField": "title" },
          "description": { "$stepField": "description" }
        },
        "outputMap": {
          "editRequested": {}
        }
      }
    ],
    "reviewTemplate": [
      {
        "title": "Product",
        "editStepId": "subtype",
        "items": [
          { "label": "Productgroep", "source": { "$literal": "Houten kozijnen" } },
          { "label": "Subtype", "source": { "$draft": "subtypeId", "format": "option-label", "stepId": "subtype" } },
          { "label": "Preset", "source": { "$draft": "presetId", "format": "option-label", "stepId": "preset" } },
          { "label": "Houtsoort", "source": { "$draft": "__woodLabel" } },
          { "label": "Glas", "source": { "$draft": "glazing" } }
        ]
      },
      {
        "title": "Maatvoering",
        "editStepId": "width",
        "items": [
          { "label": "Breedtes", "source": { "$draft": "__widthSummary" } },
          { "label": "Hoogtes", "source": { "$draft": "__heightSummary" } },
          { "label": "Vakverdeling", "source": { "$draft": "__paneSummary" } }
        ]
      },
      {
        "title": "Specificaties",
        "editStepId": "specs",
        "items": [
          { "label": "Profielmaat", "source": { "$draft": "profile", "format": "mm" } },
          { "label": "Onderdorpel", "source": { "$draft": "onderdorpel", "format": "boolean" } },
          { "label": "Ventilatierooster", "source": { "$draft": "ventilatierooster", "format": "boolean" } }
        ]
      }
    ],
    "payloadSchema": {
      "productGroup": "houten-kozijnen",
      "categoryField": "subtypeId",
      "categoryLabelFallback": "Houten kozijnen",
      "measurementFields": [
        { "key": "widthTop", "label": "Breedte boven", "unit": "mm", "draftField": "widthTop" },
        { "key": "widthMiddle", "label": "Breedte midden", "unit": "mm", "draftField": "widthMiddle" },
        { "key": "widthBottom", "label": "Breedte onder", "unit": "mm", "draftField": "widthBottom" },
        { "key": "heightLeft", "label": "Hoogte links", "unit": "mm", "draftField": "heightLeft" },
        { "key": "heightCenter", "label": "Hoogte midden", "unit": "mm", "draftField": "heightCenter" },
        { "key": "heightRight", "label": "Hoogte rechts", "unit": "mm", "draftField": "heightRight" }
      ]
    }
  }'::jsonb
);

-- ─── Tuinpoorten (3 steps) ──────────────────────────────────────────────────
INSERT INTO rac_product_flows (organization_id, product_group_id, definition) VALUES (
  NULL,
  'tuinpoorten',
  '{
    "steps": [
      {
        "id": "frame",
        "type": "single-select-cards",
        "title": "Wordt het kozijn meegenomen?",
        "description": "Geef aan of het kozijn mee wordt opgenomen.",
        "options": [
          { "id": "inclusief-kozijn", "label": "Inclusief kozijn", "description": "Deur en kozijn worden samen opgenomen als complete combinatie.", "imagePath": "/intake/door-frame-options/inclusief-kozijn.svg", "available": true },
          { "id": "exclusief-kozijn", "label": "Exclusief kozijn", "description": "Alleen de deur of het deurblad wordt opgenomen, zonder kozijn.", "imagePath": "/intake/door-frame-options/exclusief-kozijn.svg", "available": true }
        ],
        "visibleWhen": null,
        "completeWhen": { "op": "truthy", "field": "frameOptionId" },
        "inputMap": {
          "totalSteps": { "$meta": "totalSteps" },
          "categoryLabel": { "$literal": "Tuinpoorten" },
          "showFutureFlowIndicator": { "$literal": false },
          "frameOptions": { "$stepOptions": true },
          "selectedFrameOptionId": { "$draft": "frameOptionId" }
        },
        "autoAdvance": true,
        "outputMap": {
          "frameOptionSelected": {
            "patchDraft": { "frameOptionId": { "$value": true } }
          }
        }
      },
      {
        "id": "media",
        "type": "media-upload",
        "title": "Leg de situatie vast",
        "description": "Maak foto\u0027s van de huidige poort, doorgang, bevestigingspunten en eventuele obstakels rond montage of afwerking.",
        "visibleWhen": { "op": "truthy", "field": "frameOptionId" },
        "completeWhen": { "op": "truthy", "field": "__always" },
        "inputMap": {
          "stepNumber": { "$meta": "stepNumber" },
          "totalSteps": { "$meta": "totalSteps" },
          "title": { "$stepField": "title" },
          "description": { "$stepField": "description" }
        },
        "outputMap": {
          "filesChanged": { "mediaFiles": true }
        }
      },
      {
        "id": "review",
        "type": "review-summary",
        "title": "Controleer de poortopname",
        "description": "Bekijk de gekozen uitvoering voordat je deze opname bij de afspraak opslaat.",
        "visibleWhen": { "op": "truthy", "field": "frameOptionId" },
        "completeWhen": null,
        "inputMap": {
          "stepNumber": { "$meta": "stepNumber" },
          "totalSteps": { "$meta": "totalSteps" },
          "title": { "$stepField": "title" },
          "description": { "$stepField": "description" }
        },
        "outputMap": {
          "editRequested": {}
        }
      }
    ],
    "reviewTemplate": [
      {
        "title": "Selectie",
        "editStepId": "frame",
        "items": [
          { "label": "Productgroep", "source": { "$literal": "Tuinpoorten" } },
          { "label": "Categorie", "source": { "$literal": "Tuinpoorten" } },
          { "label": "Kozijn", "source": { "$draft": "frameOptionId", "format": "option-label", "stepId": "frame" } }
        ]
      }
    ],
    "payloadSchema": {
      "productGroup": "tuinpoorten",
      "categoryField": "__tuinpoortenCategory",
      "categoryLabelFallback": "Tuinpoorten",
      "frameOptionField": "frameOptionId",
      "frameOptionLabelFallback": "Niet gekozen",
      "measurementFields": []
    }
  }'::jsonb
);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DELETE FROM rac_product_flows WHERE organization_id IS NULL;
-- +goose StatementEnd
