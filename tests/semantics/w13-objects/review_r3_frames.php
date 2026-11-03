<?php
// REQ-018/020; T-035/036/039/040. Validate before callee locals unwind.
class W13R3FrameMarker {function __destruct(){$GLOBALS['r3_frame_events'][]='local';if($GLOBALS['r3_frame_errors'])throw new Exception('local');}}
class W13R3FrameText {
    function __toString():string{$GLOBALS['r3_frame_events'][]='convert';if($GLOBALS['r3_frame_errors'])throw new Exception('conversion');return 'text';}
    function __destruct(){$GLOBALS['r3_frame_events'][]='temporary';}
}
class W13R3FrameProducer {
    function value():string{$local=new W13R3FrameMarker;return new W13R3FrameText;}
}
class W13R3FrameStaticProducer {
    static function value():string{$local=new W13R3FrameMarker;return new W13R3FrameText;}
}
class W13R3FrameReferenceProducer {
    function &value():string{$local=new W13R3FrameMarker;$source=new W13R3FrameText;$alias=&$source;return $alias;}
}
foreach(['instance','static','reference','errors'] as $mode){
    $tests['r3-frame-return-'.$mode]=function()use($mode){
        $GLOBALS['r3_frame_events']=[];$GLOBALS['r3_frame_errors']=$mode==='errors';
        try{if($mode==='static')$result=(new W13R3FrameStaticProducer)->value();elseif($mode==='reference')$result=(new W13R3FrameReferenceProducer)->value();else$result=(new W13R3FrameProducer)->value();}
        catch(Throwable $caught){$result=[];do{$result[]=[$caught::class,$caught->getMessage()];}while($caught=$caught->getPrevious());}
        $GLOBALS['r3_frame_events'][]='after';
        return [$result,$GLOBALS['r3_frame_events']];
    };
}
