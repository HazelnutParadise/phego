<?php
// REQ-018/020/021; T-035/036/039-042. Initializers retain their PHP owners.
class W13Captured {
    public mixed $self;
    public function __destruct(){$GLOBALS['capture_events'][]='dtor';}
}
$tests['lazy-captured-cycle']=function(){
    $GLOBALS['capture_events']=[];$real=new W13Captured;$real->self=$real;$weak=WeakReference::create($real);
    $lazy=(new ReflectionClass(W13Captured::class))->newLazyProxy(function()use($real){return $real;});unset($real);
    $first=[gc_collect_cycles(),$weak->get()!==null];unset($lazy);
    return [$first,gc_collect_cycles(),$weak->get()===null,$GLOBALS['capture_events']];
};
$tests['lazy-capture-release']=function(){
    $GLOBALS['capture_events']=[];$capture=new W13Captured;$weak=WeakReference::create($capture);
    $lazy=(new ReflectionClass(W13Lazy::class))->newLazyGhost(function($o)use($capture){$o->i=1;});unset($capture);
    $before=$weak->get()!==null;$i=$lazy->i;return [$before,$i,$weak->get()===null,$GLOBALS['capture_events']];
};
$tests['lazy-capture-failure']=function(){
    $GLOBALS['capture_events']=[];$capture=new W13Captured;$weak=WeakReference::create($capture);
    $lazy=(new ReflectionClass(W13Lazy::class))->newLazyGhost(function($o)use($capture){throw new Exception('retry');});unset($capture);
    $e=attempt(fn()=>$lazy->i);$alive=$weak->get()!==null;unset($lazy);return [$e,$alive,$weak->get()===null,$GLOBALS['capture_events']];
};
