<?php
// exception_spl_hierarchy: instanceof matrix for the SPL exception classes against Exception, Logic, Runtime and BadFunctionCall
function row($label, $o) {
    echo $label, ": E=", ($o instanceof Exception ? 1 : 0),
        " L=", ($o instanceof LogicException ? 1 : 0),
        " R=", ($o instanceof RuntimeException ? 1 : 0),
        " BF=", ($o instanceof BadFunctionCallException ? 1 : 0),
        "\n";
}

echo "columns: E=Exception L=LogicException R=RuntimeException BF=BadFunctionCallException\n";
row("LogicException", new LogicException("x"));
row("BadFunctionCallException", new BadFunctionCallException("x"));
row("BadMethodCallException", new BadMethodCallException("x"));
row("DomainException", new DomainException("x"));
row("InvalidArgumentException", new InvalidArgumentException("x"));
row("LengthException", new LengthException("x"));
row("OutOfRangeException", new OutOfRangeException("x"));
row("RuntimeException", new RuntimeException("x"));
row("OutOfBoundsException", new OutOfBoundsException("x"));
row("OverflowException", new OverflowException("x"));
row("RangeException", new RangeException("x"));
row("UnderflowException", new UnderflowException("x"));
row("UnexpectedValueException", new UnexpectedValueException("x"));

$bad = new BadMethodCallException("method");
echo "bad_method_message: ", $bad->getMessage(), "\n";
echo "bad_method_is_bad_function: ", ($bad instanceof BadFunctionCallException ? 1 : 0), "\n";
echo "bad_method_class: ", $bad::class, "\n";
echo "end\n";
