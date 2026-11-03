<?php
// Diagnostic only: an upstream crash is not a compatibility pass.
#[AllowDynamicProperties] class Rescue {
    public function __destruct() {
        $GLOBALS['saved']=$GLOBALS['weak']->get();
        echo 'saved=',is_object($GLOBALS['saved'])?'live':'null',"\n";
    }
}
$a=new Rescue;$a->self=$a;
$b=new stdClass;$b->self=$b;
$weak=WeakReference::create($b);
unset($a,$b);
var_dump(gc_collect_cycles(),is_object($saved),$weak->get()===$saved,isset($saved->self));
