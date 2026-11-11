<?php
// REQ-018/020; T-035/036/039/040. Hold the receiver through setter argument conversion.
$tests ??= [];
class W13R12HeldReceiver {
    public int $x=0 {set(int|string $v){global$w13R12HeldEvents;$w13R12HeldEvents[]='body';$this->x=is_string($v)?strlen($v):$v;$w13R12HeldEvents[]=['stored',$this->x];}}
    function __destruct(){global$w13R12HeldEvents;$w13R12HeldEvents[]='receiver';}
}
class W13R12HeldInput {
    function __toString():string {
        global$w13R12HeldEvents,$w13R12HeldOwner;$w13R12HeldEvents[]='convert';
        try{$alias=&$w13R12HeldOwner->x;$w13R12HeldEvents[]='reference-allowed';}catch(Throwable$e){$w13R12HeldEvents[]=[$e::class,$e->getMessage()];}
        unset($GLOBALS['w13R12HeldOwner']);return'abc';
    }
    function __destruct(){global$w13R12HeldEvents;$w13R12HeldEvents[]='rhs';}
}
$tests['r12-setter-held-receiver']=function(){
    global$w13R12HeldEvents;$w13R12HeldEvents=[];$GLOBALS['w13R12HeldOwner']=new W13R12HeldReceiver;
    try{$GLOBALS['w13R12HeldOwner']->x=new W13R12HeldInput;$error=null;}catch(Throwable$e){$error=[$e::class,$e->getMessage()];}
    $w13R12HeldEvents[]='after';return[$error,$w13R12HeldEvents];
};
if(realpath($_SERVER['SCRIPT_FILENAME']??'')===__FILE__) {
    $diagnostics=[];set_error_handler(function($l,$m)use(&$diagnostics){$diagnostics[]=[$l,$m];return true;});$result=$tests['r12-setter-held-receiver']();restore_error_handler();
    echo json_encode(['php'=>PHP_VERSION,'int_size'=>PHP_INT_SIZE,'cases'=>['r12-setter-held-receiver'=>['result'=>$result,'diagnostics'=>$diagnostics]]],JSON_PRETTY_PRINT|JSON_THROW_ON_ERROR),"\n";
}
