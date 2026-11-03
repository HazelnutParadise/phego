<?php
// REQ-018/020; T-035/036/039/040. Typed reference returns never coerce sources.
class W13R3TypedReturnText {function __toString():string{$GLOBALS['r3_typed_events'][]='convert';return 'text';}}
foreach(['string','float','text'] as $target){foreach(['int','union','mixed','untyped'] as $source){
    if($target==='text'&&in_array($source,['int','union'],true))continue;
    $typ=match($source){'untyped'=>'','union'=>$target==='float'?'int|float':'int|string',default=>$source};
    $returnType=$target==='text'?'string':$target;
    foreach(['method','hook'] as $path){
        $class='W13R3TypedReturn_'.$target.'_'.$source.'_'.$path;
        $member=$path==='method'?'public function &value():'.$returnType.'{return $this->storage;}':'public '.$returnType.' $value {&get{return $this->storage;}}';
        eval('class '.$class.' {public '.$typ.' $storage;'.$member.'}');
        foreach(['','-frame'] as $frame){
            $tests['r3-typed-return-'.$target.'-'.$source.'-'.$path.$frame]=function()use($class,$target,$path){
                $GLOBALS['r3_typed_events']=[];$o=new $class;$o->storage=$target==='text'?new W13R3TypedReturnText:7;
                $result=attempt(fn()=>$path==='method'?$o->value():$o->value);
                return [$result,get_debug_type($o->storage),$o->storage,$GLOBALS['r3_typed_events']];
            };
        }
    }
}}
