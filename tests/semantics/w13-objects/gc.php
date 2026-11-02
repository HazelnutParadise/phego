<?php
// REQ-020/021; T-039-042. Observe ordering, nested GC and the second pass.
#[AllowDynamicProperties] class W13GC {
    public function __construct(public string $name) {}
    public function __destruct() {
        $GLOBALS['gc_events'][]=$this->name;
        if ($this->name==='nested') $GLOBALS['gc_events'][]=['nested',gc_collect_cycles()];
        if ($this->name==='create') { $b=new self('created');$b->self=$b; }
    }
}
$tests['gc-hole-order']=function(){
    $GLOBALS['gc_events']=[];
    $a=new W13GC('a');$a->self=$a;
    $b=new W13GC('b');$b->self=$b;
    $c=new W13GC('c');$c->self=$c;
    $w=WeakReference::create($a);
    unset($a,$b,$c);
    $w->get()->self=null;
    $n=gc_collect_cycles();
    return [$n,$GLOBALS['gc_events']];
};
$tests['gc-nested']=function(){
    $GLOBALS['gc_events']=[];
    $o=new W13GC('nested');$o->self=$o;unset($o);
    $n=gc_collect_cycles();return [$n,$GLOBALS['gc_events']];
};
$tests['gc-created']=function(){
    $GLOBALS['gc_events']=[];
    $o=new W13GC('create');$o->self=$o;unset($o);
    $first=gc_collect_cycles();$a=$GLOBALS['gc_events'];
    $second=gc_collect_cycles();return [$first,$a,$second,$GLOBALS['gc_events']];
};
$tests['lazy-uninitialized-destructor']=function(){
    $GLOBALS['gc_events']=[];
    $r=new ReflectionClass(W13GC::class);
    $o=$r->newLazyGhost(function($o){$o->name='initialized';});
    unset($o);
    return $GLOBALS['gc_events'];
};
class W13HookDtor { public function __destruct() {$GLOBALS['gc_events'][]='dtor';} }
class W13HookFrame { public int $value { get {$local=new W13HookDtor;return 1;} } }
$tests['hook-frame-lifetime']=function(){
    $GLOBALS['gc_events']=[];$o=new W13HookFrame;$v=$o->value;
    $GLOBALS['gc_events'][]='after';return [$v,$GLOBALS['gc_events']];
};
