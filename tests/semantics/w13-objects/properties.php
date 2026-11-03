<?php
// REQ-018/020; T-035/036/039/040. Reentry, overloads and cloning.
class W13HookOwner {
    public int $value { get { $GLOBALS['property_events'][]='get';unset($GLOBALS['hook_owner']);$GLOBALS['property_events'][]=($GLOBALS['hook_weak']->get()!==null?'live':'dead');return 1; } }
    public function __destruct() {$GLOBALS['property_events'][]='dtor';}
}
$tests['hook-receiver-owner']=function(){
    $GLOBALS['property_events']=[];$GLOBALS['hook_owner']=new W13HookOwner;$GLOBALS['hook_weak']=WeakReference::create($GLOBALS['hook_owner']);
    $v=$GLOBALS['hook_owner']->value;$GLOBALS['property_events'][]='after';return [$v,$GLOBALS['property_events']];
};
class W13Clone {
    public readonly int $i;
    public function __construct() {$this->i=1;}
    public function __clone() {$this->i=2;$GLOBALS['property_events'][]='clone';}
}
$tests['clone-method']=function(){ $GLOBALS['property_events']=[];$a=new W13Clone;$b=clone $a;return [$a->i,$b->i,$GLOBALS['property_events']]; };
class W13Magic {
    private array $storage=[];
    public function __get($name) { $GLOBALS['property_events'][]='get:'.$name;return $this->storage[$name]??null; }
    public function __set($name,$value) { $GLOBALS['property_events'][]='set:'.$name;$this->storage[$name]=$value; }
    public function __isset($name):bool { $GLOBALS['property_events'][]='isset:'.$name;return isset($this->storage[$name]); }
    public function __unset($name):void { $GLOBALS['property_events'][]='unset:'.$name;unset($this->storage[$name]); }
}
$tests['magic-properties']=function(){
    $GLOBALS['property_events']=[];$o=new W13Magic;$o->x=3;$a=isset($o->x);$b=$o->x;unset($o->x);$c=isset($o->x);return [$a,$b,$c,$GLOBALS['property_events']];
};
class W13ReferenceReentry {
    public function __destruct() { $GLOBALS['property_events'][]=attempt(function(){ $GLOBALS['property_alias']='x'; }); }
}
$tests['reference-bind-reentry']=function(){
    $GLOBALS['property_events']=[];$o=new W13Base;$o->i=1;$GLOBALS['property_alias']=new W13ReferenceReentry;
    $GLOBALS['property_alias'] =& $o->i;return [$o->i,$GLOBALS['property_events']];
};
$tests['property-leading-numeric']=function(){ $o=new W13Base;return attempt(function() use($o){$o->i='12x';}); };
$tests['lazy-ghost-return']=function(){
    $r=new ReflectionClass(W13Lazy::class);$o=$r->newLazyGhost(fn($o)=>1);
    return [attempt(fn()=>$o->i),$r->isUninitializedLazyObject($o)];
};
class W13LazyVirtual {public int $v {get=>42;} public int $i;}
$tests['lazy-virtual-get']=function(){
    $calls=0;$r=new ReflectionClass(W13LazyVirtual::class);$o=$r->newLazyGhost(function($o)use(&$calls){$calls++;$o->i=1;});
    return [$o->v,$calls,$r->isUninitializedLazyObject($o)];
};
class W13LazyRollback {public int $i;public mixed $child;}
class W13LazyRollbackChild {public function __destruct(){ $GLOBALS['property_events'][]=attempt(fn()=>$GLOBALS['lazy_rollback']->i); }}
$tests['lazy-rollback-reentry']=function(){
    $GLOBALS['property_events']=[];$calls=0;$r=new ReflectionClass(W13LazyRollback::class);
    $GLOBALS['lazy_rollback']=$r->newLazyGhost(function($o)use(&$calls){$calls++;$o->i=1;$o->child=new W13LazyRollbackChild;throw new Exception('retry');});
    $e=attempt(fn()=>$GLOBALS['lazy_rollback']->i);return [$e,$calls,$GLOBALS['property_events']];
};
class W13Asymmetric {public private(set) int $x=1;}
$tests['asymmetric-reference']=function(){ $o=new W13Asymmetric;return attempt(function()use($o){$r=&$o->x;}); };
$tests['asymmetric-unset']=function(){ $o=new W13Asymmetric;return attempt(function()use($o){unset($o->x);}); };
class W13LazyMethod {public int $i;public function ping(){return 7;}}
$tests['lazy-method-no-initialize']=function(){
    $calls=0;$r=new ReflectionClass(W13LazyMethod::class);$o=$r->newLazyGhost(function($o)use(&$calls){$calls++;$o->i=1;});
    return [$o->ping(),$calls,$r->isUninitializedLazyObject($o)];
};
$tests['lazy-comparison']=function(){
    $calls=0;$r=new ReflectionClass(W13Lazy::class);
    $a=$r->newLazyGhost(function($o)use(&$calls){$calls++;$o->i=1;});$b=$r->newLazyGhost(function($o)use(&$calls){$calls++;$o->i=2;});
    return [$a==$b,$calls];
};
class W13ReferenceHook {
    private $storage=1;
    public int $p { &get {return $this->storage;} }
}
$tests['hook-reference']=function(){ $o=new W13ReferenceHook;$r=&$o->p;$r=2;return [$r,$o->p]; };
$tests['hook-reference-type-source']=function(){ $o=new W13ReferenceHook;$r=&$o->p;$r='abc';return [$r,attempt(fn()=>$o->p)]; };
class W13CloneLifetime {
    public int $n=1;
    public function __clone(){unset($GLOBALS['clone_owner']);$this->n=2;$GLOBALS['property_events'][]='clone-body';}
    public function __destruct(){$GLOBALS['property_events'][]='dtor:'.$this->n;}
}
$tests['clone-source-owner']=function(){
    $GLOBALS['property_events']=[];$GLOBALS['clone_owner']=new W13CloneLifetime;
    $copy=clone $GLOBALS['clone_owner'];$GLOBALS['property_events'][]='after';return $GLOBALS['property_events'];
};
