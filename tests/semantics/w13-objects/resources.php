<?php
// REQ-019/020; T-037/038/039/040. A test-only stream has no external I/O.
class W13Stream {
    public $context;
    public function stream_open($path,$mode,$options,&$opened_path) { return true; }
    public function stream_close() {$GLOBALS['resource_events'][]='close';}
}
stream_wrapper_register('w13',W13Stream::class);
$tests['resource-last-owner']=function(){
    $GLOBALS['resource_events']=[];$a=fopen('w13://test','r');$b=$a;
    unset($a);$before=$GLOBALS['resource_events'];unset($b);
    return [$before,$GLOBALS['resource_events']];
};
$tests['resource-closed-kind']=function(){
    $GLOBALS['resource_events']=[];$a=fopen('w13://test','r');
    $open=is_resource($a);$truth=(bool)$a;fclose($a);
    return [$open,$truth,get_debug_type($a),is_resource($a),(bool)$a,$GLOBALS['resource_events']];
};
$tests['resource-property-lifetime']=function(){
    $GLOBALS['resource_events']=[];$o=new stdClass;$o->stream=fopen('w13://test','r');unset($o);
    return $GLOBALS['resource_events'];
};
