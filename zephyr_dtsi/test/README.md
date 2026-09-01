# `zephyr_dtsi` Test & Example Directory

This directory contains reference example files for validating and testing the `zephyr_dtsi` EC Devicetree generator.

---

## Files

- **`ExampleEcPinout.csv`**: Sample hardware EC pinout CSV spreadsheet containing signal names, direction, internal pull-ups/pull-downs, voltage levels, pin type overrides, enum names, and variant pinout columns (`NPCX99 (NPCX)`, `IT82002 (ITE)`).
- **`generated.dtsi`**: Reference Devicetree overlay output generated from `ExampleEcPinout.csv` using the Nuvoton NPCX9 (`npcx99`) chip definition and `--pin-col="NPCX99 (NPCX)"`.

---

## Generation Commands

### 1. Generate Reference Devicetree (`NPCX99` Variant - Nuvoton NPCX9)

Run the following command from the repository root:

```bash
./firmweaver dtsi --chip=npcx99 \
               --csv=zephyr_dtsi/test/ExampleEcPinout.csv \
               --pin-col="NPCX99 (NPCX)" \
               --output=zephyr_dtsi/test/generated.dtsi
```

### 2. Generate Reference Devicetree (`IT82002` Variant - ITE IT82002)

To generate for an ITE-based variant from the same spreadsheet:

```bash
./firmweaver dtsi --chip=it82002 \
               --csv=zephyr_dtsi/test/ExampleEcPinout.csv \
               --pin-col="IT82002 (ITE)" \
               --output=zephyr_dtsi/test/generated_ite.dtsi
```
