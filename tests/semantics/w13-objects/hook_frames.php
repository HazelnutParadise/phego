<?php
// REQ-018/020; T-035/036/039/040. Bypass belongs to the active PHP hook frame.
class W13LazyBackedHook {
    public int $p=1 {get {$GLOBALS['hook_frames'][]='get';return $this->p;}}
}
$tests['hook-lazy-backed-frame']=function(){
    $GLOBALS['hook_frames']=[];$o=(new ReflectionClass(W13LazyBackedHook::class))->newLazyGhost(function($o){$GLOBALS['hook_frames'][]='init';$o->p=9;});
    return [$o->p,$GLOBALS['hook_frames']];
};
class W13HelperHook {
    public int $count=0;
    public int $p=7 {get {$GLOBALS['hook_frames'][]='get:'.++$this->count;if($this->count===1)return $this->helper();return $this->p;}}
    public function helper(){ $GLOBALS['hook_frames'][]='helper';return $this->p; }
}
$tests['hook-helper-frame']=function(){$GLOBALS['hook_frames']=[];$o=new W13HelperHook;return [$o->p,$o->count,$GLOBALS['hook_frames']];};
class W13OtherHook {
    public ?self $other=null;
    public int $p=7 {get {$GLOBALS['hook_frames'][]='get';return $this->other?->p??$this->p;}}
}
$tests['hook-other-receiver']=function(){$GLOBALS['hook_frames']=[];$a=new W13OtherHook;$b=new W13OtherHook;$a->other=$b;return [$a->p,$GLOBALS['hook_frames']];};
class W13HookParent {
    protected int $p=7 {get {$GLOBALS['hook_frames'][]='parent';return $this->p;}}
}
class W13HookChild extends W13HookParent {
    public int $p=7 {get {if(false)return $this->p;$GLOBALS['hook_frames'][]='child';return parent::$p::get();}}
    public function readParent(){return $this->p;}
}
$tests['hook-parent-prototype']=function(){$GLOBALS['hook_frames']=[];$o=new W13HookChild;return [$o->readParent(),$GLOBALS['hook_frames']];};
class W13ProxyHookBase {public int $q=1;public int $p {get=>7;}}
class W13ProxyHookChild extends W13ProxyHookBase {
    public int $p {get {$target=(new ReflectionClass(self::class))->initializeLazyObject($this);return $target->p;}}
}
$tests['hook-proxy-backing-receiver']=function(){
    $real=new W13ProxyHookBase;$o=(new ReflectionClass(W13ProxyHookChild::class))->newLazyProxy(fn()=>$real);$o->q;
    return attempt(fn()=>$o->p);
};
