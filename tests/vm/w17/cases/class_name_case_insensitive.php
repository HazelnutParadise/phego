<?php
class MixedCase
{
    const VALUE = "v";
    public static $count = 0;

    public static function Make()
    {
        static::$count++;
        return new self;
    }

    public function Hello()
    {
        return "hello from " . self::class . " / " . static::class;
    }
}

class SubCase extends mixedcase
{
}

$a = new MIXEDCASE;
echo $a->hello(), "\n";
echo $a->HELLO(), "\n";
echo mixedcase::make()->Hello(), "\n";
echo MIXEDCASE::VALUE, " ", mixedCase::$count, "\n";
echo (new subcase)->hello(), "\n";
echo "instanceof lower: ", ($a instanceof mixedcase ? "yes" : "no"), "\n";
$n = "MiXeDcAsE";
$b = new $n;
echo "dynamic: ", $b->hello(), "\n";
echo "class constant keeps spelling: ", mixedcase::class, " ", MIXEDCASE::class, "\n";
echo "object class keeps declaration: ", $b::class, "\n";
