<?php
// catch_engine_instantiation: Error from new on abstract classes, interfaces and traits, and calls to abstract methods
function report($label, $e)
{
    echo $label, ": ", $e::class, " | ", $e->getMessage(), " | line ", $e->getLine(), "\n";
}

interface Drawable {}

trait Helper
{
    public function help()
    {
        return "help";
    }
}

abstract class Shape
{
    abstract public static function build();

    public static function label()
    {
        return "shape";
    }
}

class Circle extends Shape
{
    public static function build()
    {
        return "circle built";
    }
}

try {
    $r = new Shape();
} catch (Error $e) {
    report("abstract_class", $e);
}

try {
    $r = new Drawable();
} catch (Error $e) {
    report("interface", $e);
}

try {
    $r = new Helper();
} catch (Error $e) {
    report("trait", $e);
}

try {
    $r = Shape::build();
} catch (Error $e) {
    report("abstract_static_call", $e);
}

echo "concrete_build: ", Circle::build(), "\n";
echo "inherited_label: ", Circle::label(), "\n";
echo "end: ok\n";
