<?php
// REQ-012/018/020/021; T-023/024/035/036/039/040/041/042.
function w13R5OperationAttempt(callable $f): mixed { try { return $f(); } catch (Throwable $e) { return [$e::class, $e->getMessage()]; } }
class W13R5Ops {
    function __call($name, $args) { $GLOBALS['w13r5_events'][]='magic'; return [$name,$args]; }
    private function hidden() { return 'hidden'; }
    function normal($first,$second=null) { return [$first,$second]; }
}
class W13R5Getter {
	public int $x { get { $GLOBALS['w13r5_gets']++; return isset($this->x) ? $this->x : 42; } }
}
class W13R5NullableGetter {
	public ?int $x { get { $GLOBALS['w13r5_gets']++; return isset($this->x) ? $this->x : 42; } }
}
class W13R5InitializedGetter {
    public int $x=7 { get { $GLOBALS['w13r5_gets']++; return isset($this->x) ? $this->x : 42; } }
}
#[AllowDynamicProperties]
class W13R5DynamicRef { public static int $s=1; }
class W13R5ProxyTarget {
    public string $name;
    function __clone() { $GLOBALS['w13r5_events'][]='clone:'.$this->name; if (!empty($GLOBALS['w13r5_chain'])) throw new Exception('clone failure',0,new RuntimeException('previous')); throw new Exception('clone'); }
    function __destruct() { $GLOBALS['w13r5_events'][]='dtor:'.$this->name; if (!empty($GLOBALS['w13r5_dtor_armed']) && $this->name==='source') throw new Exception('destructor failure'); }
}

$tests['r5-op-missing-invalid-order'] = function() { $GLOBALS['w13r5_events']=[];$o=new W13R5Ops;$r=w13R5OperationAttempt(fn()=>call_user_func_array([$o,'missing'],['first'=>1,0=>2]));return[$r,$GLOBALS['w13r5_events']]; };
$tests['r5-op-hidden-invalid-order'] = function() { $GLOBALS['w13r5_events']=[];$o=new W13R5Ops;$r=w13R5OperationAttempt(fn()=>call_user_func_array([$o,'hidden'],['first'=>1,0=>2]));return[$r,$GLOBALS['w13r5_events']]; };
$tests['r5-op-valid-positional-before-named'] = function() { $GLOBALS['w13r5_events']=[];$o=new W13R5Ops;$r=call_user_func_array([$o,'missing'],[1,'second'=>2]);return$r; };
$tests['r5-op-named-only'] = function() { $GLOBALS['w13r5_events']=[];$o=new W13R5Ops;$r=call_user_func_array([$o,'missing'],['first'=>1]);return$r; };
$tests['r5-op-ordinary-positional'] = function() { $o=new W13R5Ops;return$o->normal(1,2); };
$tests['r5-op-ordinary-invalid-order'] = function() { $o=new W13R5Ops;return w13R5OperationAttempt(fn()=>call_user_func_array([$o,'normal'],['first'=>1,0=>2])); };
$tests['r5-op-getter-uninitialized'] = function() { $GLOBALS['w13r5_gets']=0;$o=new W13R5Getter;$v=$o->x;$i=isset($o->x);return[$v,$i,$GLOBALS['w13r5_gets']]; };
$tests['r5-op-getter-null'] = function() { $GLOBALS['w13r5_gets']=0;$o=new W13R5NullableGetter;$o->x=null;$v=$o->x;$i=isset($o->x);return[$v,$i,$GLOBALS['w13r5_gets']]; };
$tests['r5-op-getter-initialized'] = function() { $GLOBALS['w13r5_gets']=0;$o=new W13R5InitializedGetter;$v=$o->x;$i=isset($o->x);return[$v,$i,$GLOBALS['w13r5_gets']]; };
$tests['r5-op-proxy-clone-throws'] = function() { $GLOBALS['w13r5_events']=[];$GLOBALS['w13r5_dtor_armed']=false;$target=new W13R5ProxyTarget;$target->name='source';$r=new ReflectionClass(W13R5ProxyTarget::class);$proxy=$r->newLazyProxy(function()use($target){$GLOBALS['w13r5_events'][]='factory';return$target;});$obs=w13R5OperationAttempt(fn()=>clone$proxy);$events=$GLOBALS['w13r5_events'];$GLOBALS['w13r5_dtor_armed']=false;unset($proxy,$target);return[$obs,$events,null]; };
$tests['r5-op-ordinary-clone-failure'] = function() { $GLOBALS['w13r5_events']=[];$GLOBALS['w13r5_dtor_armed']=false;$o=new W13R5ProxyTarget;$o->name='source';$obs=w13R5OperationAttempt(fn()=>clone$o);$events=$GLOBALS['w13r5_events'];unset($o);return[$obs,$events,null]; };
$tests['r5-op-proxy-clone-dtor-chain'] = function() { $GLOBALS['w13r5_events']=[];$GLOBALS['w13r5_chain']=true;$GLOBALS['w13r5_dtor_armed']=true;$target=new W13R5ProxyTarget;$target->name='source';$r=new ReflectionClass(W13R5ProxyTarget::class);$proxy=$r->newLazyProxy(function()use($target){$GLOBALS['w13r5_events'][]='factory';return$target;});$thrown=null;$previous=[];try{clone$proxy;}catch(Throwable$e){$thrown=[$e::class,$e->getMessage()];for($p=$e->getPrevious();$p;$p=$p->getPrevious())$previous[]=[$p::class,$p->getMessage()];}$events=$GLOBALS['w13r5_events'];$GLOBALS['w13r5_dtor_armed']=false;$GLOBALS['w13r5_chain']=false;unset($proxy,$target);return[$thrown,$events,$previous?:null]; };
$tests['r5-op-clone-source-reference'] = function() { $o=new W13R5Typed;$r=&$o->x;$c=clone$o;unset($o->x);$bad=w13R5OperationAttempt(function()use(&$r){$r='bad';});return[$bad,$r]; };
$tests['r5-op-clone-reference-detach'] = function() { $o=new W13R5Typed;$r=&$o->x;$c=clone$o;unset($c->x);$bad=w13R5OperationAttempt(function()use(&$r){$r='bad';});unset($o->x);$write=w13R5OperationAttempt(function()use(&$r){$r='free';return'written';});return[$bad,$write,$r]; };
class W13R5Typed { public int $x=1; }
$tests['r5-op-clone-dynamic-reference'] = function() { $o=new W13R5DynamicRef;$o->s=5;$r=&$o->s;$c=clone$o;$write=w13R5OperationAttempt(function()use(&$r){$r='free';return'written';});return[$write,get_object_vars($c),$r]; };
