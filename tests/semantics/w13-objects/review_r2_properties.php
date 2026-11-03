<?php
class W13R2PReadonlyCoercion {
    public readonly string $text;
    public function write($value){$this->text=$value;}
}
class W13R2PReadonlyString {
    public function __construct(public $owner){}
    public function __toString():string{$this->owner->write('inner');return 'outer';}
}
$tests['r2-property-readonly-coercion-reentry']=function(){
    $o=new W13R2PReadonlyCoercion;
    $error=attempt(fn()=>$o->write(new W13R2PReadonlyString($o)));
    return [$error,$o->text];
};
class W13R2PUnset {
    public int $p = 1;
    public $u = 'initial';
    public int $q;
    public function __get($name) { $GLOBALS['events'][]='get'; return 2; }
    public function __set($name,$value) { $GLOBALS['events'][]='set'; }
    public function __isset($name) { $GLOBALS['events'][]='isset'; return true; }
    public function __unset($name) { $GLOBALS['events'][]='unset'; }
    public function __clone() { $GLOBALS['events'][]='clone'; }
}
class W13R2PReceiver {
    public string $text;
    public function __toString() { $GLOBALS['events'][]='string'; unset($GLOBALS['receiver'],$GLOBALS['source']); return 'x'; }
    public function __destruct() { $GLOBALS['events'][]='destruct'; }
}
class W13R2PReadonlyBase {
    public readonly int $p;
    public function clear() { unset($this->p); }
    public function init() { $this->p=7; }
}
class W13R2PReadonlyChild extends W13R2PReadonlyBase { public function clearChild() { unset($this->p); } }
class W13R2PResult {
    public function __destruct() {
        $GLOBALS['events'][]='result-dtor';
        if (empty($GLOBALS['inDestructor']) && isset($GLOBALS['magic'])) {
            $GLOBALS['inDestructor']=true;
            unset($GLOBALS['magic']->x);
        }
        throw new Exception('dtor');
    }
}
class W13R2PPlainResult { public function __destruct() { $GLOBALS['events'][]='plain-dtor'; } }
class W13R2PMagic {
    public function __set($name,$value) { $GLOBALS['events'][]='set'; return new W13R2PResult; }
    public function __unset($name) { $GLOBALS['events'][]='unset'; return new W13R2PResult; }
    public function __isset($name) { $GLOBALS['events'][]='isset'; return $name==='truthy' ? new W13R2PPlainResult : new W13R2PResult; }
}
$tests['r2-property-unset-magic'] = function() {
    $GLOBALS['events']=[]; $o=new W13R2PUnset; unset($o->p);
    $read=attempt(fn()=>$o->p); $write=attempt(function()use($o){$o->p=3;});
    $isset=attempt(fn()=>isset($o->p)); $missing=attempt(fn()=>$o->q);
    unset($o->p); $again=$GLOBALS['events'];
    unset($o->u); $untyped=attempt(fn()=>$o->u); $clone=clone $o; $cloneRead=attempt(fn()=>$clone->u);
    return [$read,$write,$isset,$missing,$again,$untyped,$cloneRead,$GLOBALS['events']];
};
$tests['r2-property-stringable-receiver-release'] = function() {
    $GLOBALS['events']=[]; $GLOBALS['receiver']=new W13R2PReceiver; $GLOBALS['source']=new W13R2PReceiver;
    $GLOBALS['receiver']->text=''; $GLOBALS['source']->text='source';
    $result=attempt(function(){ $GLOBALS['receiver']->text=$GLOBALS['source']; });
    return [$result,$GLOBALS['events']];
};
$tests['r2-property-readonly-unset'] = function() {
    $globalObject=new W13R2PReadonlyChild; $global=attempt(function()use($globalObject){unset($globalObject->p);});
    $declaringObject=new W13R2PReadonlyChild; $declaring=attempt(fn()=>$declaringObject->clear());
    $childObject=new W13R2PReadonlyChild; $child=attempt(fn()=>$childObject->clearChild());
    $init=attempt(fn()=>$declaringObject->init()); $initialized=attempt(fn()=>$declaringObject->clear());
    return [$global,$declaring,$child,$init,$initialized];
};
$tests['r2-property-magic-result-destruction'] = function() {
    $GLOBALS['events']=[]; $GLOBALS['inDestructor']=false; $o=new W13R2PMagic; $GLOBALS['magic']=$o;
    $set=attempt(function()use($o){$o->x=1;}); $unset=attempt(function()use($o){unset($o->x);});
    $isset=attempt(fn()=>isset($o->x));
    $truthy=attempt(fn()=>isset($o->truthy));
    unset($GLOBALS['magic']);
    return [$set,$unset,$isset,$truthy,$GLOBALS['events']];
};
