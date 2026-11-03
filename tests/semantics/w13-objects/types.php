<?php
// REQ-012/018; T-023/024/035/036. Weak checks differ from explicit casts.
foreach (['nan'=>NAN,'inf'=>INF,'huge'=>1e30,'negative-huge'=>-1e30,'string-huge'=>'1e30','fraction'=>1.5,'string-fraction'=>'1.5','numeric'=>'7'] as $label=>$input) {
    $tests['property-coerce-'.$label]=function()use($input){$o=new W13Base;return attempt(function()use($o,$input){$o->i=$input;return $o->i;});};
}
class W13BadGetter {public int $i {get {return 'abc';}}}
class W13NumericGetter {public int $i {get {return '7';}}}
$tests['getter-return-type']=function(){return attempt(fn()=>(new W13BadGetter)->i);};
$tests['getter-return-coerce']=function(){return (new W13NumericGetter)->i;};
$tests['reference-weak-coerce']=function(){$o=new W13Base;$o->i=1;$r=&$o->i;$r='7';return [$r,$o->i];};
class W13Stringable {public function __toString():string{return 'abc';}}
class W13TypedMethods {
    public function string(string $s):string{return $s;}
    public function callback(callable $c):string{return 'called';}
}
$tests['method-stringable']=function(){return (new W13TypedMethods)->string(new W13Stringable);};
foreach (['missing'=>'does_not_exist','empty'=>[],'plain'=>new stdClass] as $label=>$input) {
    $tests['callable-'.$label]=function()use($input){$r=new ReflectionMethod(W13TypedMethods::class,'callback');return attempt(fn()=>$r->invoke(new W13TypedMethods,$input));};
}
class W13Callable {
    public function __invoke(){}
    public function instance(){}
    public static function stat(){}
}
foreach(['invoke'=>new W13Callable,'method'=>[new W13Callable,'instance'],'static'=>'W13Callable::stat'] as $label=>$input){
    $tests['callable-'.$label]=function()use($input){$r=new ReflectionMethod(W13TypedMethods::class,'callback');return $r->invoke(new W13TypedMethods,$input);};
}
