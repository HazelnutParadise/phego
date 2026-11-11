<?php
// REQ-018/020; T-035/036/039/040. Slot resolution ends before old-dst destruction.
#[AllowDynamicProperties]
class W13R6GuardLifetime {
    public int $spare;
    public function &__get($name): mixed {
        $proxy=$this===$GLOBALS['r6gl_proxy'];
        $GLOBALS['r6gl_events'][]=$proxy?'get:proxy':'get:target';
        if($proxy){$GLOBALS['r6gl_dst'] =& $this->$name;return $GLOBALS['r6gl_dst'];}
        return $GLOBALS['r6gl_answer'];
    }
}
class W13R6GuardLifetimeOld {
    public function __destruct() {
        $GLOBALS['r6gl_events'][]='dtor';
        unset($GLOBALS['r6gl_target']->x);
        $value=$GLOBALS['r6gl_target']->x;
        $GLOBALS['r6gl_events'][]='value:'.$value;
    }
}
$tests['r6-proxy-guard-reference-lifetime']=function(){
    $GLOBALS['r6gl_events']=[];$GLOBALS['r6gl_answer']=23;
    $target=new W13R6GuardLifetime;$GLOBALS['r6gl_target']=$target;
    $GLOBALS['r6gl_proxy']=(new ReflectionClass(W13R6GuardLifetime::class))->newLazyProxy(function()use($target){$GLOBALS['r6gl_events'][]='init';return $target;});
    $GLOBALS['r6gl_dst']=new W13R6GuardLifetimeOld;
    $out =& $GLOBALS['r6gl_proxy']->x;
    $result=[$GLOBALS['r6gl_events'],$out,$GLOBALS['r6gl_dst'],$GLOBALS['r6gl_answer'],property_exists($target,'x')];
    unset($GLOBALS['r6gl_proxy'],$GLOBALS['r6gl_target'],$GLOBALS['r6gl_dst'],$GLOBALS['r6gl_answer']);
    return $result;
};
