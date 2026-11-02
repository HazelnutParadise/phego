<?php
// REQ-012/018; T-023/024/035/036. Call and class-state boundaries.
interface W13Interface { public function hello(): string; }
trait W13Trait { public function hello(): string { return 'trait'; } }
class W13Methods implements W13Interface {
    use W13Trait;
    public static int $counter=0;
    public function sum(int $a,int $b=2):int { return $a+$b; }
    public function bump(int &$value):int { return ++$value; }
    private function hidden():string { return 'hidden'; }
    protected function guarded():string { return 'guarded'; }
    public function __call(string $name,array $args):string { return $name.':'.json_encode($args); }
}
class W13PlainMethods { private function hidden():void {} }
$tests['trait-interface']=function(){ $o=new W13Methods;return [$o->hello(),$o instanceof W13Interface]; };
$tests['method-case']=function(){ $o=new W13Methods;return $o->HeLlO(); };
$tests['method-named']=function(){ $o=new W13Methods;return $o->sum(b:4,a:3); };
$tests['method-default']=function(){ $o=new W13Methods;return $o->sum(a:3); };
$tests['method-missing']=function(){ $o=new W13Methods;return attempt(fn()=>$o->sum()); };
$tests['method-type']=function(){ $o=new W13Methods;return attempt(fn()=>$o->sum(a:'x')); };
$tests['method-unknown-name']=function(){ $o=new W13Methods;return attempt(fn()=>$o->sum(unknown:1)); };
$tests['method-duplicate-name']=function(){ $o=new W13Methods;return attempt(fn()=>$o->sum(1,a:2)); };
$tests['method-reference']=function(){ $o=new W13Methods;$v=1;return [$o->bump($v),$v]; };
$tests['method-reference-literal']=function(){ $o=new W13Methods;return attempt(fn()=>$o->bump(1)); };
$tests['method-magic']=function(){ $o=new W13Methods;return [$o->missing(1,2),$o->hidden()]; };
$tests['method-private']=function(){ $o=new W13PlainMethods;return attempt(fn()=>$o->hidden()); };
$tests['static-state']=function(){W13Methods::$counter=3;$a=W13Methods::$counter;W13Methods::$counter=0;return $a;};
class W13MethodLifecycle {
    public function bad():int { return new W13HookDtor; }
    public function variadic(&...$args):array { $args[0]=9;return $args; }
    public function weakReturn():int { return '7'; }
    public function allArgs($a):array { return func_get_args(); }
}
$tests['method-return-cleanup']=function(){
    $GLOBALS['gc_events']=[];$o=new W13MethodLifecycle;
    $e=attempt(fn()=>$o->bad());$GLOBALS['gc_events'][]='after';return [$e,$GLOBALS['gc_events']];
};
$tests['method-variadic-reference']=function(){ $o=new W13MethodLifecycle;$v=1;$a=$o->variadic($v);return [$a,$v]; };
$tests['method-return-declaration']=function(){ $o=new W13MethodLifecycle;return $o->weakReturn(); };
$tests['method-extra-arguments']=function(){ $o=new W13MethodLifecycle;return $o->allArgs(1,2,3); };
class W13MethodResult {public function __destruct(){$GLOBALS['method_events'][]='result';}}
class W13MethodFail {
    public function done(){unset($GLOBALS['method_owner']);return new W13MethodResult;}
    public function __destruct(){$GLOBALS['method_events'][]='receiver';throw new Exception('cleanup');}
}
$tests['method-failed-result']=function(){
    $GLOBALS['method_events']=[];$GLOBALS['method_owner']=new W13MethodFail;
    try{$GLOBALS['method_owner']->done();}catch(Throwable $e){$GLOBALS['method_events'][]='caught:'.$e->getMessage();}
    $GLOBALS['method_events'][]='after';return $GLOBALS['method_events'];
};
