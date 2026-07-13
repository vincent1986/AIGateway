-- Create first GitHub Wiki page via Safari (user must be logged into GitHub)
on run argv
	set pageName to "Home"
	set pageBody to "AIGateway Wiki bootstrap. Full FAQ will be synced next."
	if (count of argv) ≥ 1 then set pageName to item 1 of argv
	if (count of argv) ≥ 2 then set pageBody to item 2 of argv

	tell application "Safari"
		activate
		set theURL to "https://github.com/vincent1986/AIGateway/wiki/_new"
		if (count of windows) = 0 then
			make new document with properties {URL:theURL}
		else
			set URL of current tab of window 1 to theURL
		end if
		delay 5
	end tell

	-- Build JS carefully
	set js to my buildJS(pageName, pageBody)

	tell application "Safari"
		try
			set resultText to do JavaScript js in current tab of window 1
			return resultText
		on error errMsg
			return "JS_ERROR: " & errMsg
		end try
	end tell
end run

on escapeJS(s)
	set s to my replaceText(s, "\\", "\\\\")
	set s to my replaceText(s, "\"", "\\\"")
	set s to my replaceText(s, return, "\\n")
	set s to my replaceText(s, linefeed, "\\n")
	return s
end escapeJS

on replaceText(theText, searchString, replacementString)
	set AppleScript's text item delimiters to searchString
	set theItems to text items of theText
	set AppleScript's text item delimiters to replacementString
	set theText to theItems as text
	set AppleScript's text item delimiters to ""
	return theText
end replaceText

on buildJS(pageName, pageBody)
	set n to my escapeJS(pageName)
	set b to my escapeJS(pageBody)
	return "(function(){var name='" & n & "';var body='" & b & "';function fill(el,v){if(!el)return;el.focus();el.value=v;el.dispatchEvent(new Event('input',{bubbles:true}));el.dispatchEvent(new Event('change',{bubbles:true}));}var nameEl=document.querySelector('input[name=\"wiki[name]\"]')||document.querySelector('#wiki_name')||document.querySelector('input#gollum-editor-page-name')||document.querySelector('input[name=\"wiki_title\"]');var contentEl=document.querySelector('textarea[name=\"wiki[body]\"]')||document.querySelector('#wiki_body')||document.querySelector('#gollum-editor-body')||document.querySelector('textarea[name=\"wiki_contents\"]')||document.querySelector('textarea');var inputs=Array.from(document.querySelectorAll('input,textarea,button,select')).slice(0,50).map(function(e){return {tag:e.tagName,name:e.name,id:e.id,type:e.type||'',text:(e.innerText||e.value||'').slice(0,50)};});fill(nameEl,name);fill(contentEl,body);var btn=document.querySelector('button[type=\"submit\"]')||Array.from(document.querySelectorAll('button,input[type=\"submit\"]')).find(function(x){return /save|create|提交/i.test((x.innerText||x.value||''));});if(btn){btn.click();}return JSON.stringify({url:location.href,title:document.title,nameEl:!!nameEl,contentEl:!!contentEl,clicked:!!btn,inputs:inputs});})();"
end buildJS
