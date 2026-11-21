<?php
interface Config
{
    const MODE = "interface";
    const LEVEL = 1;
    final const FIXED = "fixed";
}

class Overrides implements Config
{
    const MODE = "class";
}

class Inherits implements Config
{
    public function show()
    {
        return self::MODE . " " . static::LEVEL . " " . self::FIXED;
    }
}

class GrandChild extends Overrides
{
    const LEVEL = 3;
    public function show()
    {
        return self::MODE . " " . self::LEVEL . " " . parent::LEVEL . " " . self::FIXED;
    }
}

echo Config::MODE, "\n";
echo Overrides::MODE, " ", Overrides::LEVEL, "\n";
echo (new Inherits)->show(), "\n";
echo (new GrandChild)->show(), "\n";

interface Derived extends Config
{
    const MODE = "derived interface";
}
class FromDerived implements Derived
{
}
echo Derived::MODE, " ", FromDerived::MODE, " ", FromDerived::FIXED, "\n";

interface A1
{
    const SHARED = "same";
}
interface A2 extends A1
{
}
class BothPaths implements A1, A2
{
}
echo BothPaths::SHARED, "\n";
