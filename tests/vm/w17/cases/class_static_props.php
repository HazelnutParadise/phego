<?php
class Base
{
    public static $shared = 0;
    public static $own = "base own";
    public static $list = [];
    protected static $hidden = "hidden";

    public static function incShared()
    {
        return ++self::$shared;
    }

    public static function ownViaStatic()
    {
        return static::$own;
    }

    public static function hidden()
    {
        return static::$hidden;
    }
}

class Child extends Base
{
    public static $own = "child own";
    protected static $hidden = "child hidden";
}

Base::incShared();
Child::incShared();
Child::$shared++;
echo "shared: ", Base::$shared, " ", Child::$shared, "\n";
echo "own: ", Base::$own, " | ", Child::$own, "\n";
echo "late static: ", Base::ownViaStatic(), " | ", Child::ownViaStatic(), "\n";
echo "protected late static: ", Base::hidden(), " | ", Child::hidden(), "\n";

Child::$own = "changed child";
echo "redeclared independent: ", Base::$own, " | ", Child::$own, "\n";

Base::$list[] = "a";
Child::$list[] = "b";
Base::$list["k"]["j"] = "deep";
foreach (Child::$list as $k => $v) {
    echo "list ", $k, " => ", ($k === "k" ? $v["j"] : $v), "\n";
}

$ref = &Base::$shared;
$ref = 40;
echo "via reference: ", Child::$shared, "\n";
Base::$shared += 2;
echo "compound: ", $ref, "\n";
Child::$shared .= "!";
echo "concat: ", Base::$shared, "\n";

$cls = "Child";
echo "dynamic class: ", $cls::$own, "\n";
$o = new Child;
echo "via object: ", $o::$own, "\n";

echo "isset: ", (isset(Base::$own) ? "yes" : "no"), " ", (isset(Base::$nope) ? "yes" : "no"), " ", (isset(Base::$hidden) ? "yes" : "no"), "\n";
Base::$own = null;
echo "isset null: ", (isset(Base::$own) ? "yes" : "no"), "\n";
echo "empty: ", (empty(Base::$list) ? "yes" : "no"), "\n";
Base::$own ??= "defaulted";
echo "coalesce assign: ", Base::$own, "\n";

class Typed
{
    public static int $n = 1;
    public static ?string $s = null;
}
Typed::$n = "42";
Typed::$n++;
echo "typed static: ", Typed::$n, " ", (Typed::$s ?? "null"), "\n";
