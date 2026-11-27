<?php
// catch_engine_visibility: Error for calling or reading private and protected members from global scope
function report($label, $e)
{
    echo $label, ": ", $e::class, " | ", $e->getMessage(), " | line ", $e->getLine(), "\n";
}

class Vis
{
    const PUB = 1;
    private const PRIV = 2;
    private $secret = 1;
    protected $guarded = 2;
    public $open = 3;

    private function priv()
    {
        return "p";
    }

    protected function prot()
    {
        return "q";
    }

    protected static function pstat()
    {
        return "s";
    }

    public function inside()
    {
        return $this->priv() . $this->prot() . $this->secret . self::PRIV;
    }
}

class Single
{
    private function __construct()
    {
    }

}

$v = new Vis();

try {
    $r = $v->priv();
} catch (Error $e) {
    report("private_method", $e);
}

try {
    $r = $v->prot();
} catch (Error $e) {
    report("protected_method", $e);
}

try {
    $r = Vis::pstat();
} catch (Error $e) {
    report("protected_static", $e);
}

try {
    $r = $v->secret;
} catch (Error $e) {
    report("private_property", $e);
}

try {
    $r = $v->guarded;
} catch (Error $e) {
    report("protected_property", $e);
}

try {
    $r = Vis::PRIV;
} catch (Error $e) {
    report("private_constant", $e);
}

try {
    $r = new Single();
} catch (Error $e) {
    report("private_constructor", $e);
}

echo "inside: ", $v->inside(), "\n";
echo "end: ok\n";
