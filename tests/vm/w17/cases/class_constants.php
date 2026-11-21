<?php
const GLOBAL_BASE = 100;

class Limits
{
    const MIN = 1;
    const MAX = self::MIN + 9;
    const RANGE = [self::MIN, self::MAX];
    const LABEL = "limits " . self::MIN . "-" . self::MAX;
    const FROM_GLOBAL = GLOBAL_BASE * 2;
    const NESTED = ["a" => ["b" => self::MAX]];
    public const PUB = "public";
    protected const PROT = "protected";
    private const PRIV = "private";
    const LATER = self::DEFINED_AFTER . "!";
    const DEFINED_AFTER = "after";

    public function inside()
    {
        return self::PRIV . " " . self::PROT . " " . static::PROT;
    }
}

class Extended extends Limits
{
    const MAX = 50;
    protected const PROT = "child protected";

    public function show()
    {
        return parent::MAX . " " . self::MAX . " " . static::MAX . " " . parent::PROT . " " . self::PROT;
    }
}

echo Limits::MIN, " ", Limits::MAX, "\n";
echo Limits::RANGE[0], ",", Limits::RANGE[1], "\n";
echo Limits::LABEL, "\n";
echo Limits::FROM_GLOBAL, "\n";
echo Limits::NESTED["a"]["b"], "\n";
echo Limits::PUB, "\n";
echo Limits::LATER, "\n";
echo (new Limits)->inside(), "\n";
echo (new Extended)->inside(), "\n";
echo (new Extended)->show(), "\n";
echo "inherited computed: ", Extended::LABEL, "\n";
$o = new Extended;
echo "via instance: ", $o::MAX, " ", $o::MIN, "\n";
$cls = "Extended";
echo "via string: ", $cls::MAX, "\n";
echo "constant class name: ", Limits::class, "\n";
echo "isset on constant array: ", (isset(Limits::RANGE[1]) ? "yes" : "no"), (isset(Limits::RANGE[5]) ? "yes" : "no"), "\n";
