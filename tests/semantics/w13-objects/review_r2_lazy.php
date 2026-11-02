<?php
class W13R2ZeroProperties {public function __destruct(){$GLOBALS['r2_zero_events'][]='destructor';}}
foreach([false,true] as $proxy){
    $tests['r2-lazy-zero-'.($proxy?'proxy':'ghost')]=function()use($proxy){
        $GLOBALS['r2_zero_events']=[];
        $r=new ReflectionClass(W13R2ZeroProperties::class);
        $factory=function(){$GLOBALS['r2_zero_events'][]='initializer';return new W13R2ZeroProperties;};
        $initializer=function(){$GLOBALS['r2_zero_events'][]='initializer';};
        $o=$proxy?$r->newLazyProxy($factory):$r->newLazyGhost($initializer);
        $before=[$r->isUninitializedLazyObject($o),$r->getLazyInitializer($o)===null];
        unset($o);
        return [$before,$GLOBALS['r2_zero_events']];
    };
}
class W13R2Captured {
    public function __destruct(){
        $GLOBALS['r2_zero_events'][]='capture';
        if($GLOBALS['r2_capture_throw'])throw new Exception('capture');
    }
}
function w13R2CapturedInitializer($proxy){
    $captured=new W13R2Captured;
    return $proxy?function()use($captured){return new W13R2ZeroProperties;}:function()use($captured){};
}
foreach([false,true] as $proxy)foreach([false,true] as $throw){
    $tests['r2-lazy-capture-'.($throw?'throw-':'').($proxy?'proxy':'ghost')]=function()use($proxy,$throw){
        $GLOBALS['r2_zero_events']=[];$GLOBALS['r2_capture_throw']=$throw;
        $r=new ReflectionClass(W13R2ZeroProperties::class);
        $flags=attempt(function()use($r,$proxy){
            $initializer=w13R2CapturedInitializer($proxy);
            $o=$proxy?$r->newLazyProxy($initializer):$r->newLazyGhost($initializer);
            unset($initializer);
            $flags=[$r->isUninitializedLazyObject($o),$r->getLazyInitializer($o)===null];
            $GLOBALS['r2_zero_events'][]='after-constructor';
            unset($o);
            return $flags;
        });
        return [$flags,$GLOBALS['r2_zero_events']];
    };
}
class W13R2CompareLazy {public int $a;public int $b;}
$tests['r2-lazy-comparison-state']=function(){
    $r=new ReflectionClass(W13R2CompareLazy::class);
    $left=$r->newLazyGhost(function($o){$o->a=3;});
    $right=new W13R2CompareLazy;$right->a=3;$right->b=2;
    return [$left<=>$right,$left<=>$right];
};
