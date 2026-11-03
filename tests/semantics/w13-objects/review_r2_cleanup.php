<?php
// REQ-018/020; T-035/036/039/040. Consume callback results at their use site.
class W13R2CleanupChild {
    public function __destruct(){
        $GLOBALS['r2_cleanup'][]='child';
        if($GLOBALS['r2_cleanup_throw'])throw new Exception('child');
    }
}
class W13R2CleanupHook {public object $p {get {return new W13R2CleanupChild;}}}
class W13R2CleanupClone {
    public function __clone(){
        switch($GLOBALS['r2_cleanup_mode']){
            case 'self':return $this;
            case 'array':return [$this];
            case 'double':throw new Exception('clone');
            default:return new W13R2CleanupChild;
        }
    }
    public function __destruct(){
        $GLOBALS['r2_cleanup'][]='clone';
        if($GLOBALS['r2_cleanup_mode']==='double')throw new Exception('destructor');
    }
}
#[AllowDynamicProperties] class W13R2CleanupDynamic {
    public int $storage=7;
    public function &__get($name){$GLOBALS['r2_cleanup'][]='get';return $this->storage;}
}
class W13R2CleanupParent {
    private function __clone(){$GLOBALS['r2_cleanup'][]='parent';}
    public function duplicate(){return clone $this;}
}
class W13R2CleanupSubclass extends W13R2CleanupParent {
    public function __clone(){$GLOBALS['r2_cleanup'][]='subclass';}
}
class W13R2CleanupReentrantChild {
    public function __construct(public W13R2CleanupReentrant $owner){}
    public function __destruct(){$GLOBALS['r2_cleanup'][]='child';$this->owner->initialize();}
}
class W13R2CleanupReentrant {
    public readonly string $text;
    public function __construct(){$this->text='before';}
    public function initialize(){$this->text='after';}
    public function __clone(){return new W13R2CleanupReentrantChild($this);}
}
foreach(['isset','isset-throw','clone','clone-throw','clone-self','clone-array','clone-double','dynamic','handler','reentry'] as $mode){
    $tests['r2-cleanup-'.$mode]=function()use($mode){
        $GLOBALS['r2_cleanup']=[];
        $GLOBALS['r2_cleanup_throw']=str_ends_with($mode,'throw');
        $GLOBALS['r2_cleanup_mode']=str_starts_with($mode,'clone-')?substr($mode,6):'';
        $observed=null;
        try {
            if(str_starts_with($mode,'isset')){$o=new W13R2CleanupHook;$observed=isset($o->p);}
            elseif($mode==='dynamic'){$o=new W13R2CleanupDynamic;$o->x=1;$alias=&$o->x;$alias=2;$observed=[$o->x,$o->storage];unset($alias);}
            elseif($mode==='handler'){$o=new W13R2CleanupSubclass;$copy=$o->duplicate();unset($copy);}
            elseif($mode==='reentry'){$o=new W13R2CleanupReentrant;$copy=clone $o;$observed=$copy->text;unset($copy);}
            else {$o=new W13R2CleanupClone;$copy=clone $o;$GLOBALS['r2_cleanup'][]='after-clone';unset($copy);}
        } catch(Throwable $e){$observed=[$e::class,$e->getMessage(),$e->getPrevious()?->getMessage()];}
        $GLOBALS['r2_cleanup'][]='after';
        // Snapshot before the original receiver's normal scope cleanup.
        $result=[$observed,$GLOBALS['r2_cleanup']];
        $GLOBALS['r2_cleanup_mode']='';
        return $result;
    };
}
