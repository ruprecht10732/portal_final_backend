The logic behind the wizard flow in the `MeasurementStart` component contains a subtle but critical bug: it mixes reactive step calculations via an Angular `effect` with manual step advancement (`nextStep()`).

Because `currentStep` was automatically syncing its value via an `effect` reacting to state changes (like selecting a door category or frame option), it would instantly jump ahead. However, the selection methods *also* use a 180ms `setTimeout` to manually call `nextStep()`, which increments the step once more. This causes the wizard to completely skip intermediate steps (like the frame option for outer doors) and breaks navigation. Additionally, the `setTimeout` wrapper selectively skipped advancing for non-interior doors, leaving the user stuck on step 2 for front/garden doors.

Here are the fixes to ensure production-grade wizard navigation:
1. **Initialize instead of sync:** We'll remove the `effect` responsible for syncing `currentStep` and explicitly set it once in the `constructor()`. This preserves proper hydration (restoring the step when refreshing or returning to the flow) while allowing `nextStep()` and `prevStep()` to freely control the UI.
2. **Remove selective advancement:** We'll clean up the 180ms `setTimeout` condition in `selectDoorFrameOption()` so that non-interior doors can properly advance to their final review step.
3. **Align component tests:** We'll adjust the test suite so it constructs the component *after* mocking the required `IntakeFlowService` state to accurately verify hydration, and ensure it respects the `setTimeout` timing.

### 1. Fix the Component Logic
`src/app/features/intake/pages/measurement-start/measurement-start.ts`

```typescript
<<<<
  constructor() {
    effect(() => {
      this.currentStep.set(this.resolveCurrentStep());
    });

    effect(() => {
====
  constructor() {
    this.currentStep.set(this.resolveCurrentStep());

    effect(() => {
>>>>
```

```typescript
<<<<
  protected selectDoorFrameOption(doorFrameOptionId: string): void {
    const doorFrameOption = this.doorFrameOptions.find((option) => option.id === doorFrameOptionId);

    if (!doorFrameOption?.available) {
      return;
    }

    this.intakeFlow.setSelectedDoorFrameOption(doorFrameOptionId);
    this.clearSubmissionFeedback();

    if (this.selectedDoorCategoryId() === 'binnendeuren') {
      globalThis.setTimeout(() => {
        this.nextStep();
      }, 180);
    }
  }
====
  protected selectDoorFrameOption(doorFrameOptionId: string): void {
    const doorFrameOption = this.doorFrameOptions.find((option) => option.id === doorFrameOptionId);

    if (!doorFrameOption?.available) {
      return;
    }

    this.intakeFlow.setSelectedDoorFrameOption(doorFrameOptionId);
    this.clearSubmissionFeedback();

    globalThis.setTimeout(() => {
      this.nextStep();
    }, 180);
  }
>>>>
```

### 2. Update the Test Suite
`src/app/features/intake/pages/measurement-start/measurement-start.spec.ts`

```typescript
<<<<
  beforeEach(async () => {
    visitReportServiceMock = {
      existingMeasurements: null,
      existingMeasurementProducts: [],
      upsertCalls: [],
      getVisitReport: async (appointmentId: string) => visitReportServiceMock.existingMeasurements === null
        ? null
        : {
            appointmentId,
            measurements: visitReportServiceMock.existingMeasurements,
            measurementProducts: visitReportServiceMock.existingMeasurementProducts,
            notes: null,
            accessDifficulty: null,
            createdAt: new Date().toISOString(),
            updatedAt: new Date().toISOString(),
          },
      upsertVisitReport: async (appointmentId: string, input: { measurements?: string; measurementProducts?: ReadonlyArray<VisitReportMeasurementProduct>; notes?: string }) => {
        visitReportServiceMock.upsertCalls.push({ appointmentId, input });
        visitReportServiceMock.existingMeasurements = input.measurements ?? null;
        visitReportServiceMock.existingMeasurementProducts = input.measurementProducts ?? [];

        return {
          appointmentId,
          measurements: input.measurements ?? null,
          measurementProducts: input.measurementProducts ?? [],
          notes: input.notes ?? null,
          accessDifficulty: null,
          createdAt: new Date().toISOString(),
          updatedAt: new Date().toISOString(),
        };
      },
    };

    await TestBed.configureTestingModule({
      imports: [MeasurementStart, TestRouteComponent],
      providers: [
        { provide: IntakeVisitReportService, useValue: visitReportServiceMock },
        provideRouter([
          { path: 'intake/start', component: TestRouteComponent },
          { path: 'intake/service-selection', component: TestRouteComponent },
        ]),
      ],
    }).compileComponents();

    intakeFlowService = TestBed.inject(IntakeFlowService);
    fixture = TestBed.createComponent(MeasurementStart);
    component = fixture.componentInstance;
    fixture.detectChanges();
    await fixture.whenStable();
  });

  it('should create', () => {
    expect(component).toBeTruthy();
  });

  it('builds preview geometry from dimensions, dividers, and vent settings', () => {
    const measurementStart = component as MeasurementStart & {
====
  beforeEach(async () => {
    visitReportServiceMock = {
      existingMeasurements: null,
      existingMeasurementProducts: [],
      upsertCalls: [],
      getVisitReport: async (appointmentId: string) => visitReportServiceMock.existingMeasurements === null
        ? null
        : {
            appointmentId,
            measurements: visitReportServiceMock.existingMeasurements,
            measurementProducts: visitReportServiceMock.existingMeasurementProducts,
            notes: null,
            accessDifficulty: null,
            createdAt: new Date().toISOString(),
            updatedAt: new Date().toISOString(),
          },
      upsertVisitReport: async (appointmentId: string, input: { measurements?: string; measurementProducts?: ReadonlyArray<VisitReportMeasurementProduct>; notes?: string }) => {
        visitReportServiceMock.upsertCalls.push({ appointmentId, input });
        visitReportServiceMock.existingMeasurements = input.measurements ?? null;
        visitReportServiceMock.existingMeasurementProducts = input.measurementProducts ?? [];

        return {
          appointmentId,
          measurements: input.measurements ?? null,
          measurementProducts: input.measurementProducts ?? [],
          notes: input.notes ?? null,
          accessDifficulty: null,
          createdAt: new Date().toISOString(),
          updatedAt: new Date().toISOString(),
        };
      },
    };

    await TestBed.configureTestingModule({
      imports: [MeasurementStart, TestRouteComponent],
      providers: [
        { provide: IntakeVisitReportService, useValue: visitReportServiceMock },
        provideRouter([
          { path: 'intake/start', component: TestRouteComponent },
          { path: 'intake/service-selection', component: TestRouteComponent },
        ]),
      ],
    }).compileComponents();

    intakeFlowService = TestBed.inject(IntakeFlowService);
  });

  async function createComponent() {
    fixture = TestBed.createComponent(MeasurementStart);
    component = fixture.componentInstance;
    fixture.detectChanges();
    await fixture.whenStable();
  }

  it('should create', async () => {
    await createComponent();
    expect(component).toBeTruthy();
  });

  it('builds preview geometry from dimensions, dividers, and vent settings', async () => {
    await createComponent();
    const measurementStart = component as MeasurementStart & {
>>>>
```

```typescript
<<<<
  it('shows the door category flow for deuren', async () => {
    intakeFlowService.setSelectedService('deuren');

    fixture.detectChanges();
    await fixture.whenStable();

    const nativeElement = fixture.nativeElement as HTMLElement;
====
  it('shows the door category flow for deuren', async () => {
    intakeFlowService.setSelectedService('deuren');
    await createComponent();

    const nativeElement = fixture.nativeElement as HTMLElement;
>>>>
```

```typescript
<<<<
  it('advances to the frame step after selecting a door category', async () => {
    intakeFlowService.setSelectedService('deuren');
    fixture.detectChanges();
    await fixture.whenStable();

    (component as MeasurementStart & {
      selectDoorCategory: (doorCategoryId: string) => void;
    }).selectDoorCategory('buitendeuren');

    fixture.detectChanges();
    await fixture.whenStable();

    const nativeElement = fixture.nativeElement as HTMLElement;
====
  it('advances to the frame step after selecting a door category', async () => {
    intakeFlowService.setSelectedService('deuren');
    await createComponent();

    (component as MeasurementStart & {
      selectDoorCategory: (doorCategoryId: string) => void;
    }).selectDoorCategory('buitendeuren');

    fixture.detectChanges();
    await fixture.whenStable();
    fixture.detectChanges();

    const nativeElement = fixture.nativeElement as HTMLElement;
>>>>
```

```typescript
<<<<
  it('hydrates the frame step when a saved door category exists', async () => {
    intakeFlowService.setSelectedService('deuren');
    intakeFlowService.setSelectedDoorCategory('tuindeuren');

    fixture.detectChanges();
    await fixture.whenStable();

    const nativeElement = fixture.nativeElement as HTMLElement;
====
  it('hydrates the frame step when a saved door category exists', async () => {
    intakeFlowService.setSelectedService('deuren');
    intakeFlowService.setSelectedDoorCategory('tuindeuren');
    await createComponent();

    const nativeElement = fixture.nativeElement as HTMLElement;
>>>>
```

```typescript
<<<<
  it('stores the selected door frame option in the shared intake flow', async () => {
    intakeFlowService.setSelectedService('deuren');
    intakeFlowService.setSelectedDoorCategory('binnendeuren');

    fixture.detectChanges();
    await fixture.whenStable();

    (component as MeasurementStart & {
      selectDoorFrameOption: (doorFrameOptionId: string) => void;
    }).selectDoorFrameOption('exclusief-kozijn');
====
  it('stores the selected door frame option in the shared intake flow', async () => {
    intakeFlowService.setSelectedService('deuren');
    intakeFlowService.setSelectedDoorCategory('binnendeuren');
    await createComponent();

    (component as MeasurementStart & {
      selectDoorFrameOption: (doorFrameOptionId: string) => void;
    }).selectDoorFrameOption('exclusief-kozijn');
>>>>
```

```typescript
<<<<
  it('advances to the binnendeur type step after selecting the frame option', async () => {
    intakeFlowService.setSelectedService('deuren');
    intakeFlowService.setSelectedDoorCategory('binnendeuren');

    fixture.detectChanges();
    await fixture.whenStable();

    (component as MeasurementStart & {
      selectDoorFrameOption: (doorFrameOptionId: string) => void;
    }).selectDoorFrameOption('inclusief-kozijn');

    fixture.detectChanges();
    await fixture.whenStable();

    const nativeElement = fixture.nativeElement as HTMLElement;
====
  it('advances to the binnendeur type step after selecting the frame option', async () => {
    intakeFlowService.setSelectedService('deuren');
    intakeFlowService.setSelectedDoorCategory('binnendeuren');
    await createComponent();

    (component as MeasurementStart & {
      selectDoorFrameOption: (doorFrameOptionId: string) => void;
    }).selectDoorFrameOption('inclusief-kozijn');

    fixture.detectChanges();
    await fixture.whenStable();
    fixture.detectChanges();

    const nativeElement = fixture.nativeElement as HTMLElement;
>>>>
```

```typescript
<<<<
  it('hydrates the binnendeur type step when binnendeur frame data exists', async () => {
    intakeFlowService.setSelectedService('deuren');
    intakeFlowService.setSelectedDoorCategory('binnendeuren');
    intakeFlowService.setSelectedDoorFrameOption('exclusief-kozijn');

    fixture.detectChanges();
    await fixture.whenStable();

    const nativeElement = fixture.nativeElement as HTMLElement;
====
  it('hydrates the binnendeur type step when binnendeur frame data exists', async () => {
    intakeFlowService.setSelectedService('deuren');
    intakeFlowService.setSelectedDoorCategory('binnendeuren');
    intakeFlowService.setSelectedDoorFrameOption('exclusief-kozijn');
    await createComponent();

    const nativeElement = fixture.nativeElement as HTMLElement;
>>>>
```

```typescript
<<<<
  it('stores the binnendeur type in the shared intake flow', async () => {
    intakeFlowService.setSelectedService('deuren');
    intakeFlowService.setSelectedDoorCategory('binnendeuren');
    intakeFlowService.setSelectedDoorFrameOption('inclusief-kozijn');

    fixture.detectChanges();
    await fixture.whenStable();

    (component as MeasurementStart & {
      selectInteriorDoorType: (interiorDoorTypeId: string) => void;
    }).selectInteriorDoorType('opdekdeuren');
====
  it('stores the binnendeur type in the shared intake flow', async () => {
    intakeFlowService.setSelectedService('deuren');
    intakeFlowService.setSelectedDoorCategory('binnendeuren');
    intakeFlowService.setSelectedDoorFrameOption('inclusief-kozijn');
    await createComponent();

    (component as MeasurementStart & {
      selectInteriorDoorType: (interiorDoorTypeId: string) => void;
    }).selectInteriorDoorType('opdekdeuren');
>>>>
```

```typescript
<<<<
  it('advances to the supplier step after selecting the binnendeur type', async () => {
    intakeFlowService.setSelectedService('deuren');
    intakeFlowService.setSelectedDoorCategory('binnendeuren');
    intakeFlowService.setSelectedDoorFrameOption('inclusief-kozijn');

    fixture.detectChanges();
    await fixture.whenStable();

    (component as MeasurementStart & {
      selectInteriorDoorType: (interiorDoorTypeId: string) => void;
    }).selectInteriorDoorType('stompe-deuren');

    fixture.detectChanges();
    await fixture.whenStable();

    const nativeElement = fixture.nativeElement as HTMLElement;
====
  it('advances to the supplier step after selecting the binnendeur type', async () => {
    intakeFlowService.setSelectedService('deuren');
    intakeFlowService.setSelectedDoorCategory('binnendeuren');
    intakeFlowService.setSelectedDoorFrameOption('inclusief-kozijn');
    await createComponent();

    (component as MeasurementStart & {
      selectInteriorDoorType: (interiorDoorTypeId: string) => void;
    }).selectInteriorDoorType('stompe-deuren');

    fixture.detectChanges();
    await fixture.whenStable();
    fixture.detectChanges();

    const nativeElement = fixture.nativeElement as HTMLElement;
>>>>
```

```typescript
<<<<
  it('hydrates the supplier step when binnendeur data exists', async () => {
    intakeFlowService.setSelectedService('deuren');
    intakeFlowService.setSelectedDoorCategory('binnendeuren');
    intakeFlowService.setSelectedDoorFrameOption('exclusief-kozijn');
    intakeFlowService.setSelectedInteriorDoorType('opdekdeuren');

    fixture.detectChanges();
    await fixture.whenStable();

    const nativeElement = fixture.nativeElement as HTMLElement;
====
  it('hydrates the supplier step when binnendeur data exists', async () => {
    intakeFlowService.setSelectedService('deuren');
    intakeFlowService.setSelectedDoorCategory('binnendeuren');
    intakeFlowService.setSelectedDoorFrameOption('exclusief-kozijn');
    intakeFlowService.setSelectedInteriorDoorType('opdekdeuren');
    await createComponent();

    const nativeElement = fixture.nativeElement as HTMLElement;
>>>>
```

```typescript
<<<<
  it('stores the supplier choice in the shared intake flow', async () => {
    intakeFlowService.setSelectedService('deuren');
    intakeFlowService.setSelectedDoorCategory('binnendeuren');
    intakeFlowService.setSelectedDoorFrameOption('inclusief-kozijn');
    intakeFlowService.setSelectedInteriorDoorType('stompe-deuren');

    fixture.detectChanges();
    await fixture.whenStable();

    (component as MeasurementStart & {
      selectDoorSupplierOption: (doorSupplierOptionId: string) => void;
    }).selectDoorSupplierOption('wij-leveren-deur');
====
  it('stores the supplier choice in the shared intake flow', async () => {
    intakeFlowService.setSelectedService('deuren');
    intakeFlowService.setSelectedDoorCategory('binnendeuren');
    intakeFlowService.setSelectedDoorFrameOption('inclusief-kozijn');
    intakeFlowService.setSelectedInteriorDoorType('stompe-deuren');
    await createComponent();

    (component as MeasurementStart & {
      selectDoorSupplierOption: (doorSupplierOptionId: string) => void;
    }).selectDoorSupplierOption('wij-leveren-deur');
>>>>
```

```typescript
<<<<
  it('advances to the door measurements step after selecting the supplier', async () => {
    intakeFlowService.setSelectedService('deuren');
    intakeFlowService.setSelectedDoorCategory('binnendeuren');
    intakeFlowService.setSelectedDoorFrameOption('inclusief-kozijn');
    intakeFlowService.setSelectedInteriorDoorType('stompe-deuren');

    fixture.detectChanges();
    await fixture.whenStable();

    (component as MeasurementStart & {
      selectDoorSupplierOption: (doorSupplierOptionId: string) => void;
    }).selectDoorSupplierOption('lead-levert-deur');

    fixture.detectChanges();
    await fixture.whenStable();

    const nativeElement = fixture.nativeElement as HTMLElement;
====
  it('advances to the door measurements step after selecting the supplier', async () => {
    intakeFlowService.setSelectedService('deuren');
    intakeFlowService.setSelectedDoorCategory('binnendeuren');
    intakeFlowService.setSelectedDoorFrameOption('inclusief-kozijn');
    intakeFlowService.setSelectedInteriorDoorType('stompe-deuren');
    await createComponent();

    (component as MeasurementStart & {
      selectDoorSupplierOption: (doorSupplierOptionId: string) => void;
    }).selectDoorSupplierOption('lead-levert-deur');

    fixture.detectChanges();
    await fixture.whenStable();
    fixture.detectChanges();

    const nativeElement = fixture.nativeElement as HTMLElement;
>>>>
```

```typescript
<<<<
  it('hydrates the door measurements step when supplier data exists', async () => {
    intakeFlowService.setSelectedService('deuren');
    intakeFlowService.setSelectedDoorCategory('binnendeuren');
    intakeFlowService.setSelectedDoorFrameOption('exclusief-kozijn');
    intakeFlowService.setSelectedInteriorDoorType('opdekdeuren');
    intakeFlowService.setSelectedDoorSupplierOption('wij-leveren-deur');

    fixture.detectChanges();
    await fixture.whenStable();

    const nativeElement = fixture.nativeElement as HTMLElement;
====
  it('hydrates the door measurements step when supplier data exists', async () => {
    intakeFlowService.setSelectedService('deuren');
    intakeFlowService.setSelectedDoorCategory('binnendeuren');
    intakeFlowService.setSelectedDoorFrameOption('exclusief-kozijn');
    intakeFlowService.setSelectedInteriorDoorType('opdekdeuren');
    intakeFlowService.setSelectedDoorSupplierOption('wij-leveren-deur');
    await createComponent();

    const nativeElement = fixture.nativeElement as HTMLElement;
>>>>
```

```typescript
<<<<
  it('advances to the customer wishes step after valid measurements when we provide the door', async () => {
    intakeFlowService.setSelectedService('deuren');
    intakeFlowService.setSelectedDoorCategory('binnendeuren');
    intakeFlowService.setSelectedDoorFrameOption('inclusief-kozijn');
    intakeFlowService.setSelectedInteriorDoorType('stompe-deuren');
    intakeFlowService.setSelectedDoorSupplierOption('wij-leveren-deur');

    fixture.detectChanges();
    await fixture.whenStable();

    const measurementStart = component as MeasurementStart & {
====
  it('advances to the customer wishes step after valid measurements when we provide the door', async () => {
    intakeFlowService.setSelectedService('deuren');
    intakeFlowService.setSelectedDoorCategory('binnendeuren');
    intakeFlowService.setSelectedDoorFrameOption('inclusief-kozijn');
    intakeFlowService.setSelectedInteriorDoorType('stompe-deuren');
    intakeFlowService.setSelectedDoorSupplierOption('wij-leveren-deur');
    await createComponent();

    const measurementStart = component as MeasurementStart & {
>>>>
```

```typescript
<<<<
  it('hydrates the customer wishes step when saved preferences exist', async () => {
    intakeFlowService.setSelectedService('deuren');
    intakeFlowService.setSelectedDoorCategory('binnendeuren');
    intakeFlowService.setSelectedDoorFrameOption('inclusief-kozijn');
    intakeFlowService.setSelectedInteriorDoorType('stompe-deuren');
    intakeFlowService.setSelectedDoorSupplierOption('wij-leveren-deur');
    intakeFlowService.setSelectedDoorMeasurements({ widthMm: 930, heightMm: 2315, thicknessMm: 114 });
    intakeFlowService.setSelectedDoorDesignPreferences({
      customerWishes: 'Ranke binnendeur met rustige profilering.',
      examplePreferenceId: 'examples-available',
      exampleNotes: 'Klant heeft een Pinterest-bord klaarstaan.',
    });
    TestBed.tick();

    fixture.detectChanges();
    await fixture.whenStable();

    const measurementStart = component as MeasurementStart & {
====
  it('hydrates the customer wishes step when saved preferences exist', async () => {
    intakeFlowService.setSelectedService('deuren');
    intakeFlowService.setSelectedDoorCategory('binnendeuren');
    intakeFlowService.setSelectedDoorFrameOption('inclusief-kozijn');
    intakeFlowService.setSelectedInteriorDoorType('stompe-deuren');
    intakeFlowService.setSelectedDoorSupplierOption('wij-leveren-deur');
    intakeFlowService.setSelectedDoorMeasurements({ widthMm: 930, heightMm: 2315, thicknessMm: 114 });
    intakeFlowService.setSelectedDoorDesignPreferences({
      customerWishes: 'Ranke binnendeur met rustige profilering.',
      examplePreferenceId: 'examples-available',
      exampleNotes: 'Klant heeft een Pinterest-bord klaarstaan.',
    });

    await createComponent();

    const measurementStart = component as MeasurementStart & {
>>>>
```

```typescript
<<<<
  it('stores the customer wishes and example preference in the shared intake flow', async () => {
    intakeFlowService.setSelectedService('deuren');
    intakeFlowService.setSelectedDoorCategory('binnendeuren');
    intakeFlowService.setSelectedDoorFrameOption('inclusief-kozijn');
    intakeFlowService.setSelectedInteriorDoorType('stompe-deuren');
    intakeFlowService.setSelectedDoorSupplierOption('wij-leveren-deur');
    intakeFlowService.setSelectedDoorMeasurements({ widthMm: 930, heightMm: 2315, thicknessMm: 114 });

    fixture.detectChanges();
    await fixture.whenStable();

    const measurementStart = component as MeasurementStart & {
====
  it('stores the customer wishes and example preference in the shared intake flow', async () => {
    intakeFlowService.setSelectedService('deuren');
    intakeFlowService.setSelectedDoorCategory('binnendeuren');
    intakeFlowService.setSelectedDoorFrameOption('inclusief-kozijn');
    intakeFlowService.setSelectedInteriorDoorType('stompe-deuren');
    intakeFlowService.setSelectedDoorSupplierOption('wij-leveren-deur');
    intakeFlowService.setSelectedDoorMeasurements({ widthMm: 930, heightMm: 2315, thicknessMm: 114 });
    await createComponent();

    const measurementStart = component as MeasurementStart & {
>>>>
```

```typescript
<<<<
  it('stores the door measurements in the shared intake flow', async () => {
    intakeFlowService.setSelectedService('deuren');
    intakeFlowService.setSelectedDoorCategory('binnendeuren');
    intakeFlowService.setSelectedDoorFrameOption('inclusief-kozijn');
    intakeFlowService.setSelectedInteriorDoorType('stompe-deuren');
    intakeFlowService.setSelectedDoorSupplierOption('wij-leveren-deur');

    fixture.detectChanges();
    await fixture.whenStable();

    const measurementStart = component as MeasurementStart & {
====
  it('stores the door measurements in the shared intake flow', async () => {
    intakeFlowService.setSelectedService('deuren');
    intakeFlowService.setSelectedDoorCategory('binnendeuren');
    intakeFlowService.setSelectedDoorFrameOption('inclusief-kozijn');
    intakeFlowService.setSelectedInteriorDoorType('stompe-deuren');
    intakeFlowService.setSelectedDoorSupplierOption('wij-leveren-deur');
    await createComponent();

    const measurementStart = component as MeasurementStart & {
>>>>
```

```typescript
<<<<
  it('clears only the current product draft when adding another product', async () => {
    const router = TestBed.inject(Router);

    intakeFlowService.setSelectedAppointment('appointment-1');
    intakeFlowService.setSelectedService('deuren');
    intakeFlowService.setSelectedDoorCategory('binnendeuren');
    intakeFlowService.setSelectedDoorFrameOption('inclusief-kozijn');
    intakeFlowService.setSelectedInteriorDoorType('stompe-deuren');

    await (component as MeasurementStart & { addAnotherProduct: () => Promise<void> }).addAnotherProduct();
====
  it('clears only the current product draft when adding another product', async () => {
    const router = TestBed.inject(Router);

    intakeFlowService.setSelectedAppointment('appointment-1');
    intakeFlowService.setSelectedService('deuren');
    intakeFlowService.setSelectedDoorCategory('binnendeuren');
    intakeFlowService.setSelectedDoorFrameOption('inclusief-kozijn');
    intakeFlowService.setSelectedInteriorDoorType('stompe-deuren');
    await createComponent();

    await (component as MeasurementStart & { addAnotherProduct: () => Promise<void> }).addAnotherProduct();
>>>>
```

```typescript
<<<<
  it('shows edit affordances on the review step and can jump back to an earlier section', async () => {
    intakeFlowService.setSelectedService('deuren');
    intakeFlowService.setSelectedDoorCategory('buitendeuren');
    intakeFlowService.setSelectedDoorFrameOption('inclusief-kozijn');

    fixture.detectChanges();
    await fixture.whenStable();

    expect((fixture.nativeElement as HTMLElement).textContent).toContain('Bewerken');
====
  it('shows edit affordances on the review step and can jump back to an earlier section', async () => {
    intakeFlowService.setSelectedService('deuren');
    intakeFlowService.setSelectedDoorCategory('buitendeuren');
    intakeFlowService.setSelectedDoorFrameOption('inclusief-kozijn');
    await createComponent();

    expect((fixture.nativeElement as HTMLElement).textContent).toContain('Bewerken');
>>>>
```

```typescript
<<<<
  it('submits structured measurement products alongside the legacy text fallback', async () => {
    intakeFlowService.setSelectedAppointment('appointment-1');
    intakeFlowService.setSelectedService('deuren');
    intakeFlowService.setSelectedDoorCategory('binnendeuren');
    intakeFlowService.setSelectedDoorFrameOption('inclusief-kozijn');
    intakeFlowService.setSelectedInteriorDoorType('stompe-deuren');
    intakeFlowService.setSelectedDoorSupplierOption('wij-leveren-deur');
    intakeFlowService.setSelectedDoorMeasurements({ widthMm: 930, heightMm: 2315, thicknessMm: 114 });
    intakeFlowService.setSelectedDoorDesignPreferences({
      customerWishes: 'Ranke binnendeur met rustige profilering.',
      examplePreferenceId: 'examples-available',
      exampleNotes: 'Pinterest-bord beschikbaar',
    });

    fixture.detectChanges();
    await fixture.whenStable();

    await (component as MeasurementStart & { submit: () => Promise<void> }).submit();
====
  it('submits structured measurement products alongside the legacy text fallback', async () => {
    intakeFlowService.setSelectedAppointment('appointment-1');
    intakeFlowService.setSelectedService('deuren');
    intakeFlowService.setSelectedDoorCategory('binnendeuren');
    intakeFlowService.setSelectedDoorFrameOption('inclusief-kozijn');
    intakeFlowService.setSelectedInteriorDoorType('stompe-deuren');
    intakeFlowService.setSelectedDoorSupplierOption('wij-leveren-deur');
    intakeFlowService.setSelectedDoorMeasurements({ widthMm: 930, heightMm: 2315, thicknessMm: 114 });
    intakeFlowService.setSelectedDoorDesignPreferences({
      customerWishes: 'Ranke binnendeur met rustige profilering.',
      examplePreferenceId: 'examples-available',
      exampleNotes: 'Pinterest-bord beschikbaar',
    });
    await createComponent();

    await (component as MeasurementStart & { submit: () => Promise<void> }).submit();
>>>>
```# Appointment Visit Report Structured Measurements Proposal

## Goal

Move appointment visit report measurements from appended plain text blocks to a structured JSON payload per measured product, while keeping the existing visit report endpoint stable during rollout.

Current limitation:

- `measurements` on `rac_appointment_visit_reports` is stored as text.
- The frontend currently appends readable sections for each measured product.
- Backend consumers can read the text, but cannot reliably filter, validate, or render per-product measurement data.

## Current API

Current request and response shape:

```json
{
  "measurements": "plain text summary",
  "accessDifficulty": "Low",
  "notes": "optional"
}
```

## Proposed additive contract

Keep the existing route:

- `GET /api/v1/appointments/:id/visit-report`
- `PUT /api/v1/appointments/:id/visit-report`

Add a new optional JSON field alongside the legacy text field.

### Request

```json
{
  "measurements": "plain text summary",
  "measurementProducts": [
    {
      "productGroup": "deuren",
      "category": "binnendeuren",
      "frameOption": "inclusief-kozijn",
      "productType": "stompe-deuren",
      "supplier": "wij-leveren-deur",
      "measurements": [
        { "key": "frameWidthMm", "label": "Kozijn breedte", "value": 930, "unit": "mm" },
        { "key": "frameHeightMm", "label": "Kozijn hoogte", "value": 2315, "unit": "mm" },
        { "key": "frameDepthMm", "label": "Kozijn diepte", "value": 114, "unit": "mm" }
      ],
      "preferences": {
        "customerWishes": "Ranke binnendeur met rustige profilering.",
        "examplePreference": "examples-available",
        "exampleNotes": "Pinterest-bord beschikbaar"
      },
      "capturedAt": "2026-03-31T14:00:00Z"
    }
  ],
  "accessDifficulty": "Low",
  "notes": "optional"
}
```

### Response

```json
{
  "appointmentId": "uuid",
  "measurements": "plain text summary",
  "measurementProducts": [
    {
      "productGroup": "deuren",
      "category": "binnendeuren",
      "frameOption": "inclusief-kozijn",
      "productType": "stompe-deuren",
      "supplier": "wij-leveren-deur",
      "measurements": [
        { "key": "frameWidthMm", "label": "Kozijn breedte", "value": 930, "unit": "mm" }
      ],
      "preferences": {
        "customerWishes": "Ranke binnendeur met rustige profilering.",
        "examplePreference": "examples-available",
        "exampleNotes": "Pinterest-bord beschikbaar"
      },
      "capturedAt": "2026-03-31T14:00:00Z"
    }
  ],
  "accessDifficulty": "Low",
  "notes": "optional",
  "createdAt": "2026-03-31T14:00:00Z",
  "updatedAt": "2026-03-31T14:10:00Z"
}
```

## Proposed backend model

Add a new JSONB column to `rac_appointment_visit_reports`:

- `measurement_products jsonb not null default '[]'::jsonb`

Keep `measurements text` during the migration window for:

- backward compatibility with existing frontend builds,
- readable exports and operational support,
- agent and audit flows that still consume plain text.

## Product payload shape

Suggested internal shape:

```json
{
  "productGroup": "deuren | houten-kozijnen",
  "category": "binnendeuren",
  "frameOption": "inclusief-kozijn",
  "productType": "stompe-deuren",
  "supplier": "wij-leveren-deur",
  "measurements": [
    {
      "key": "frameWidthMm",
      "label": "Kozijn breedte",
      "value": 930,
      "unit": "mm"
    }
  ],
  "preferences": {
    "customerWishes": "text",
    "examplePreference": "examples-available",
    "exampleNotes": "text"
  },
  "summaryText": "single product readable summary",
  "capturedAt": "timestamp"
}
```

## Rollout plan

1. Add `measurement_products` JSONB column and DTO field.
2. Accept both `measurements` and `measurementProducts` in `PUT`.
3. Populate `measurements` from `measurementProducts` server-side for backward compatibility.
4. Update frontend to submit structured products first and keep `measurements` as fallback text.
5. Migrate downstream consumers to prefer `measurementProducts`.
6. Remove the frontend-side text concatenation once all consumers are updated.

## Frontend implications

Frontend can simplify to:

- build one structured object per measured product,
- append that object to `measurementProducts`,
- derive review summaries and text export from the same structured source,
- support multiple products on one appointment without reparsing text blocks.

## Why this is preferable

- Enables reliable per-product rendering in the app.
- Supports validation and analytics by product/category/type.
- Avoids brittle text parsing in backend agents and exports.
- Preserves backward compatibility through additive rollout.