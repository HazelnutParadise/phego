<?php
// REQ-018/020; T-035/036/039/040. By-value reads consume temporary references.
class W13R2ReturnChild {
    public function __toString():string{return 'child';}
    public function __destruct(){$GLOBALS['r2_return_events'][]='child';}
}
class W13R2ReturnMagic {
    public function &__get($name){$local=new W13R2ReturnChild;return $local;}
}
class W13R2ReturnMagicScalar {public function __get($name){return 2;}}
class W13R2ReturnMagicObject {public function __get($name){return new W13R2ReturnChild;}}
class W13R2ReturnReadonly {
    public readonly int $p;
    public int $storage=1;
    public function clear(){unset($this->p);}
    public function &__get($name){return $this->storage;}
}
class W13R2ReturnHook {
    public W13R2ReturnChild $p { &get {$local=new W13R2ReturnChild;return $local;} }
}
class W13R2ReturnCoerced {
    public string $p { get {$local=new W13R2ReturnChild;$ref=&$local;return $ref;} }
}
foreach(['magic','hook','magic-frame','hook-frame','coerced-hook','coerced-hook-frame','magic-reference','hook-reference','magic-reference-frame','hook-reference-frame','magic-value-object-reference','magic-value-scalar-reference','magic-readonly-reference'] as $mode){
    $tests['r2-return-'.$mode]=function()use($mode){
        $GLOBALS['r2_return_events']=[];
        $o=str_starts_with($mode,'magic')?new W13R2ReturnMagic:(str_starts_with($mode,'coerced')?new W13R2ReturnCoerced:new W13R2ReturnHook);
        if(str_contains($mode,'value-scalar'))$o=new W13R2ReturnMagicScalar;
        if(str_contains($mode,'value-object'))$o=new W13R2ReturnMagicObject;
        if(str_contains($mode,'readonly')){$o=new W13R2ReturnReadonly;$o->clear();}
        if(str_contains($mode,'reference'))$v=&$o->p;else $v=$o->p;
        if(str_contains($mode,'scalar'))$observed=$v;
        if(str_contains($mode,'readonly')){$v=2;$observed=$o->storage;}
        unset($v);
        $GLOBALS['r2_return_events'][]='after';
        return (str_contains($mode,'scalar')||str_contains($mode,'readonly'))?[$observed,$GLOBALS['r2_return_events']]:$GLOBALS['r2_return_events'];
    };
}
