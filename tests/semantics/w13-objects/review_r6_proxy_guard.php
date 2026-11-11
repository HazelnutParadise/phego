<?php
// REQ-018/020; T-035/036/039/040. Guard ownership during proxy forwarding.
#[AllowDynamicProperties]
class W13R6ProxyGuard {
    public int $q; public int $r; public int $spare;
    public function __get($name): mixed { $GLOBALS['r6pg_events'][]='get:'.$name; if(!str_ends_with($GLOBALS['r6pg_mode'],'-dynamic-direct')) $x=$this->spare; return $this->$name; }
    public function __isset($name): bool { $GLOBALS['r6pg_events'][]='isset:'.$name; if(!str_ends_with($GLOBALS['r6pg_mode'],'-dynamic-direct')) $x=$this->spare; return isset($this->$name); }
    public function __unset($name): void { $GLOBALS['r6pg_events'][]='unset:'.$name; if(!str_ends_with($GLOBALS['r6pg_mode'],'-dynamic-direct')) $x=$this->spare; unset($this->$name); }
    public function __set($name, $value): void {
        $mode=$GLOBALS['r6pg_mode'];
        $GLOBALS['r6pg_events'][]='set:'.$name.($mode==='set-owned' ? ($this===$GLOBALS['r6pg_target'] ? ':target' : ':proxy') : '');
        if(count($GLOBALS['r6pg_events'])>12) throw new Exception('bounded reentry');
        if($mode==='set-owned' && $this===$GLOBALS['r6pg_target']) {
            $GLOBALS['r6pg_proxy']->q=$value; unset($this->q); $this->q=17; return;
        }
        if($mode==='set-different' && $name==='q') { $this->r=$value; return; }
        if($mode==='set-spare') $x=$this->spare;
        $this->$name=$value;
    }
}
foreach(['get','reference','isset','set','unset','set-spare','isset-fresh','set-owned','set-different',
        'get-dynamic','reference-dynamic','isset-dynamic','set-dynamic','unset-dynamic','get-dynamic-direct','reference-dynamic-direct','isset-dynamic-direct','unset-dynamic-direct'] as $mode) {
    $tests['r6-proxy-guard-'.$mode]=function()use($mode) {
        $GLOBALS['r6pg_events']=[]; $GLOBALS['r6pg_mode']=$mode;
        $reflection=new ReflectionClass(W13R6ProxyGuard::class);
        $target=new W13R6ProxyGuard; $target->spare=7; unset($target->q,$target->r);
        $proxy=$reflection->newLazyProxy(function()use($target){$GLOBALS['r6pg_events'][]='init';return $target;});
        $GLOBALS['r6pg_target']=$target; $GLOBALS['r6pg_proxy']=$proxy;
        $dynamic=str_contains($mode,'-dynamic');$field=$dynamic?'x':'q';
        if($mode!=='isset-fresh'&&!$dynamic) foreach(['q','r']as$raw){$reflection->getProperty($raw)->setRawValueWithoutLazyInitialization($proxy,11);unset($proxy->$raw);}
        $result=attempt(function()use($mode,$proxy,$target,$field){switch($mode){
            case 'get':case 'get-dynamic':case 'get-dynamic-direct':return $proxy->$field;
            case 'reference':case 'reference-dynamic':case 'reference-dynamic-direct':$alias=&$proxy->$field;return $alias;
            case 'isset':case 'isset-fresh':case 'isset-dynamic':case 'isset-dynamic-direct':return isset($proxy->$field);
            case 'set':case 'set-spare':case 'set-dynamic':$proxy->$field=12;return $proxy->$field;
            case 'set-owned':$target->q=12;return $target->q;
            case 'set-different':$proxy->q=12;return $proxy->r;
            case 'unset':case 'unset-dynamic':case 'unset-dynamic-direct':unset($proxy->$field);return null;
        }});
        $out=[$result,$GLOBALS['r6pg_events'],$reflection->isUninitializedLazyObject($proxy)];
        unset($GLOBALS['r6pg_target'],$GLOBALS['r6pg_proxy']);return $out;
    };
}
