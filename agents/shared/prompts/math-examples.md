=== CALCULATOR EXPRESSION EXAMPLES ===
[EXAMPLE] Material subtotal + VAT: Calculator(expression="((unit_price_1 * qty_1) + (unit_price_2 * qty_2)) * 1.21").
[EXAMPLE] Material subtotal + VAT + markup: Calculator(expression="(((unit_price_1 * qty_1) + (unit_price_2 * qty_2)) * 1.21) * 1.10").
[EXAMPLE] Fixed-size units: Calculator(expression="ceil_divide(required_amount, unit_size)").
