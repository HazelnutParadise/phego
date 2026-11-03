<?php
// REQ-018/020; T-035/036/039/040. Return coercion owns its raw getter result.
class W13R3DroppingGetter {public string $value {get {return $GLOBALS['owner'];}}}
foreach(['normal','conversion','destructor'] as $mode){foreach(['','-frame'] as $frame){
    $tests['r3-hook-drop-'.$mode.$frame]=function()use($mode){
        $GLOBALS['events']=[];$GLOBALS['failure']=$mode;$GLOBALS['owner']=new W13R3OwnedStringable;
        $previous=null;
        try{$result=(new W13R3DroppingGetter)->value;}
        catch(Throwable $caught){$result=[$caught::class,$caught->getMessage()];if($prior=$caught->getPrevious())$previous=[$prior::class,$prior->getMessage()];}
        return [$result,$GLOBALS['events'],$previous];
    };
}}
