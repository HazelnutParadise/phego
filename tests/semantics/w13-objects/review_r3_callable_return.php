<?php
// REQ-012/018; T-023/024/035/036. Callable validation does not coerce a reference.
class W13R3CallableTarget {function go(){return 'go';}function __invoke(){return 'go';}}
foreach(['array','object','invalid'] as $shape){foreach(['typed','mixed','untyped'] as $source){
    if($shape==='invalid'&&$source!=='typed')continue;
    $typ=match($source){'untyped'=>'','typed'=>$shape==='object'?'object':'array',default=>$source};
    $class='W13R3TypedCallable_'.$shape.'_'.$source;
    eval('class '.$class.' {public '.$typ.' $storage;function &value():callable{return $this->storage;}}');
    foreach(['','-frame'] as $frame){
        $tests['r3-callable-return-'.$shape.'-'.$source.$frame]=function()use($shape,$class){
            $o=new $class;$target=new W13R3CallableTarget;$o->storage=$shape==='object'?$target:[$target,$shape==='invalid'?'missing':'go'];
            try{$returned=$o->value();$result=['ok',is_callable($returned),$returned(),$returned===$o->storage];}
            catch(Throwable $caught){$result=[$caught::class,$caught->getMessage()];}
            return [$result,$o->storage];
        };
    }
}}
