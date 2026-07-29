# Tiny fixture used by the extract package's Python extractor tests. Has
# no relation to claimcheck's own functionality.


def add(a, b):
    return a + b


def sub(a, b):
    return a - b


def mul(a, b):
    return a * b


def div(a, b):
    # deliberately left untested by test_calc.py so coverage is partial,
    # not a suspicious 100%.
    if b == 0:
        raise ValueError("division by zero")
    return a / b
