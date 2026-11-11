<?php
// REQ-020/021; T-039-042. Map ownership is independent from weak keys.
$tests['weak-map-destroy']=function(){
    $GLOBALS['gc_events']=[];$m=new WeakMap;$k=new stdClass;$v=new W13GC('value');$w=WeakReference::create($v);
    $m[$k]=$v;unset($v,$m);return [$w->get()===null,$GLOBALS['gc_events']];
};
$tests['weak-map-delete']=function(){
    $GLOBALS['gc_events']=[];$m=new WeakMap;$k=new stdClass;$v=new W13GC('value');$m[$k]=$v;unset($v);
    unset($m[$k]);return [count($m),$GLOBALS['gc_events']];
};
$tests['weak-map-clone']=function(){
    $GLOBALS['gc_events']=[];$m=new WeakMap;$k=new stdClass;$v=new W13GC('value');$m[$k]=$v;unset($v);
    $clone=clone $m;unset($m);$before=$GLOBALS['gc_events'];unset($k);
    return [$before,count($clone),$GLOBALS['gc_events']];
};
$tests['weak-map-value-map-cycle']=function(){
    $m=new WeakMap;$key=new stdClass;$v=new stdClass;$v->map=$m;$m[$key]=$v;$w=WeakReference::create($m);
    unset($v,$m);$n=gc_collect_cycles();return [$n,$w->get()===null];
};
$tests['weak-map-key-map-cycle']=function(){
    $m=new WeakMap;$key=new stdClass;$v=new stdClass;$key->map=$m;$v->key=$key;$m[$key]=$v;$w=WeakReference::create($m);
    unset($key,$v,$m);$n=gc_collect_cycles();return [$n,$w->get()===null];
};
$tests['weak-map-live-key']=function(){
    $m=new WeakMap;$key=new stdClass;$v=new stdClass;$v->key=$key;$m[$key]=$v;$w=WeakReference::create($v);unset($v);
    $n=gc_collect_cycles();return [$n,count($m),$w->get()!==null];
};
$tests['weak-map-reference-clone']=function(){
    $m=new WeakMap;$key=new stdClass;$m[$key]=1;$r=&$m[$key];$clone=clone $m;
    $m[$key]=2;$before=[$r,$clone[$key]];$clone[$key]=3;$after=[$r,$m[$key]];
    unset($m[$key]);$r=4;return [$before,$after,$clone[$key],count($m)];
};
