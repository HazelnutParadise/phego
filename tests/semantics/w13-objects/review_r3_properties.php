<?php

class W13R3PrivateParent {
    private $x = 1;
    public function readParent() { return $this->x; }
    public function writeParent() { $this->x = 4; }
    public function refParent() { $r =& $this->x; $r = 6; return $this->x; }
}
#[AllowDynamicProperties]
class W13R3PrivateChild extends W13R3PrivateParent {
    public function readChild() { return $this->x; }
}

#[AllowDynamicProperties] class W13R3Lazy { public ?int $value; }
enum W13R3PureEnum { case A; case B; }
enum W13R3BackedEnum: string { case A = 'a'; case B = 'b'; }
class W13R3EnumBox {public UnitEnum $unit;public BackedEnum $backed;}
#[AllowDynamicProperties]
class W13R3Nul {
    private $x = 1;
    protected $y = 2;
    public function __get($name) { $GLOBALS['r3_nul_events'][] = 'get'; return 'magic'; }
    public function __set($name, $value) { $GLOBALS['r3_nul_events'][] = 'set'; }
    public function __isset($name) { $GLOBALS['r3_nul_events'][] = 'isset'; return true; }
    public function __unset($name) { $GLOBALS['r3_nul_events'][] = 'unset'; }
}
class W13R3StaticParent { private static $x = 1; }
class W13R3StaticChild extends W13R3StaticParent {}

$tests['r3-property-private-inherited'] = function() {
    $o = new W13R3PrivateChild;
    $read = attempt(fn() => $o->x);
    $write = attempt(function() use ($o) { $o->x = 2; });
    $ref = attempt(function() use ($o, &$r) { $r =& $o->x; return $r; });
    unset($r);
    $isset = attempt(fn() => isset($o->x));
    $unset = attempt(function() use ($o) { unset($o->x); });
    $p = new W13R3PrivateChild;
    $parentRead = attempt(fn() => $p->readParent());
    $parentWrite = attempt(fn() => $p->writeParent());
    $parentAfter = attempt(fn() => $p->readParent());
    $parentReference = attempt(fn() => $p->refParent());
    $c = new W13R3PrivateChild;
    $childScope = attempt(fn() => $c->readChild());
    $childWrite = attempt(function() use ($c) { $c->x = 9; });
    $parentStill = attempt(fn() => $c->readParent());
    $dynamic = attempt(fn() => $c->x);
    return [$read, $write, $ref, $isset, $unset, $parentRead, $parentWrite, $parentAfter, $parentReference, $childScope, $childWrite, $parentStill, $dynamic];
};

$tests['r3-property-private-static'] = function() {
    return attempt(fn() => W13R3StaticChild::$x);
};

$tests['r3-property-lazy-ghost'] = function() {
    $r = new ReflectionClass(W13R3Lazy::class);
    $calls = 0;
    $o = $r->newLazyGhost(function($o) use (&$calls) { $calls++; $o->dynamic=7; $o->value = 7; });
    $undeclared = attempt(fn() => isset($o->dynamic));
    $before = $r->isUninitializedLazyObject($o);
    $value = attempt(fn() => $o->value);
    $afterValue = $r->isUninitializedLazyObject($o);
    $n = $r->newLazyGhost(function($o) use (&$calls) { $calls++; $o->value = null; });
    $null = attempt(fn() => $n->value);
    $afterNull = $r->isUninitializedLazyObject($n);
    $m = $r->newLazyGhost(function($o) use (&$calls) { $calls++; });
    $missing = attempt(fn() => $m->value);
    $afterMissing = $r->isUninitializedLazyObject($m);
    $th = $r->newLazyGhost(function($o) use (&$calls) { $calls++; throw new Exception('initializer'); });
    $thrown = attempt(fn() => $th->value);
    $afterThrow = $r->isUninitializedLazyObject($th);
    $target = new W13R3Lazy;
    $target->dynamic = 11;
    $proxy = $r->newLazyProxy(fn() => $target);
    $proxyDynamic = attempt(fn() => $proxy->dynamic);
    return [$undeclared, $before, $value, $afterValue, $null, $afterNull, $missing, $afterMissing, $thrown, $afterThrow, $proxyDynamic, isset($proxy->dynamic), $r->isUninitializedLazyObject($proxy), $calls];
};

$tests['r3-property-enum-readonly'] = function() {
    $pure = W13R3PureEnum::A;
    $backed = W13R3BackedEnum::A;
    $probe = function($e, $properties) {
        $out = [(array)$e];
        foreach ($properties as $name) {
            $out[] = attempt(function() use ($e, $name) { $e->$name = 'changed'; });
            $out[] = attempt(function() use ($e, $name) { unset($e->$name); });
            $out[] = attempt(function() use ($e, $name, &$ref) { $ref =& $e->$name; return $ref; });
        }
        $out[] = (array)$e;
        $out[]=attempt(function()use($e){$e->extra=1;});$out[]=isset($e->extra);
        return $out;
    };
    $box=new W13R3EnumBox;$box->unit=$pure;$box->backed=$backed;
    $bad=attempt(function()use($box,$pure){$box->backed=$pure;});
    return [$probe($pure, ['name']), $probe($backed, ['name', 'value']), (array)W13R3BackedEnum::A,(array)$box->unit,(array)$box->backed,$bad];
};

$tests['r3-property-leading-nul'] = function() {
    $GLOBALS['r3_nul_events'] = [];
    $o = new W13R3Nul;
    $probe = function($name) use ($o) {
        return [
            attempt(fn() => $o->{$name}),
            attempt(function() use ($o, $name) { $o->{$name} = 8; }),
            attempt(fn() => isset($o->{$name})),
            attempt(function() use ($o, $name) { unset($o->{$name}); }),
            attempt(function() use ($o, $name, &$ref) { $ref =& $o->{$name}; return $ref; }),
        ];
    };
    return [$probe("\0x"), $probe("\0*y"), $probe('x'), $GLOBALS['r3_nul_events'], (array)$o];
};

#[AllowDynamicProperties] class W13R3NulPlain {
    private int $secret=1;protected int $p=2;
    public function secret(){return $this->secret;}
    public function p(){return $this->p;}
}
$tests['r3-property-mangled-nul']=function(){
    $out=[];
    foreach(["\0W13R3NulPlain\0secret","\0*\0p"] as $key){
        $o=new W13R3NulPlain;
        $out[]=[attempt(fn()=>$o->$key),attempt(function()use($o,$key){$o->$key='bad';}),attempt(fn()=>isset($o->$key)),attempt(function()use($o,$key){unset($o->$key);}),attempt(function()use($o,$key){$r=&$o->$key;return $r;}),$o->secret(),$o->p()];
    }
    $o=new W13R3NulPlain;
    foreach(['',"a\0b"] as $key){$o->$key='ok';$out[]=$o->$key;}
    return $out;
};
