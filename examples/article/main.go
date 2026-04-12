package main

import (
	"fmt"
	"os"

	"github.com/akkaraponph/folio"
	"github.com/akkaraponph/folio/fonts/sarabun"
	"github.com/akkaraponph/folio/thai"
)

// writer wraps the document + current page and handles page breaks.
type writer struct {
	doc       *folio.Document
	page      *folio.Page
	pageH     float64 // page height in user units (mm)
	bMargin   float64 // bottom margin
	lMargin   float64 // left margin
	bodyWidth float64 // usable width
}

func (w *writer) newPage() {
	w.page = w.doc.AddPage(folio.A4)
	w.page.SetXY(w.lMargin, 15)
}

// needsBreak checks if adding h mm of content would overflow, and if so starts a new page.
func (w *writer) needsBreak(h float64) {
	if w.page.GetY()+h > w.pageH-w.bMargin {
		w.newPage()
	}
}

func main() {
	doc := folio.New(folio.WithCompression(true))
	doc.SetTitle("ประสิทธิภาพและความเหมาะสมของภาษาโปรแกรม Go (Golang)")
	doc.SetAuthor("Folio Thai Article Example")
	doc.SetMargins(20, 15, 20)

	if err := sarabun.Register(doc); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	// Plug in the built-in Thai word segmenter so line-wrapping respects
	// word boundaries (e.g. "ต้อง", "ใช่") instead of breaking mid-word.
	thai.Setup(doc)

	// A4 is 210mm wide. With left margin 20 and right margin 20 the usable
	// width is 170; we shave an extra 5mm off the right as visual edge
	// padding so justified text never kisses the right edge.
	w := &writer{
		doc:       doc,
		pageH:     297.0,
		bMargin:   15.0,
		lMargin:   20.0,
		bodyWidth: 165.0,
	}

	// ---- Page 1: Title + Abstract ----
	w.newPage()
	p := w.page

	// Title
	doc.SetFont("sarabun", "B", 18)
	doc.SetTextColor(0, 51, 102)
	p.SetXY(w.lMargin, 20)
	p.MultiCell(w.bodyWidth, 8,
		"ประสิทธิภาพและความเหมาะสมของภาษาโปรแกรม Go (Golang) "+
			"ในการพัฒนาระบบซอฟต์แวร์สมัยใหม่", "", "C", false)

	// Horizontal rule
	p.SetY(p.GetY() + 3)
	doc.SetDrawColor(0, 51, 102)
	doc.SetLineWidth(0.5)
	p.Line(w.lMargin, p.GetY(), w.lMargin+w.bodyWidth, p.GetY())
	p.SetY(p.GetY() + 5)

	// Abstract
	w.sectionHeading("บทคัดย่อ")
	w.bodyParagraph(
		"ภาษาโปรแกรม Go หรือ Golang เป็นภาษาแบบโอเพ่นซอร์สที่พัฒนาโดยบริษัท Google " +
			"เพื่อแก้ปัญหาความซับซ้อนและข้อจำกัดของภาษาระดับระบบในยุคที่ระบบซอฟต์แวร์มีขนาดใหญ่" +
			"และต้องรองรับการทำงานพร้อมกันจำนวนมาก โดยมีจุดเด่นด้านประสิทธิภาพ " +
			"ความเรียบง่ายของไวยากรณ์ และกลไกการทำงานพร้อมกัน (concurrency) " +
			"ผ่าน goroutines และ channels")
	w.bodyParagraph(
		"บทความวิชาการฉบับนี้มีวัตถุประสงค์เพื่อศึกษา (1) แนวคิดและหลักการออกแบบของภาษา Go " +
			"(2) ภาพรวมคุณลักษณะเชิงเทคนิคที่เอื้อต่อการพัฒนาระบบซอฟต์แวร์สมัยใหม่ " +
			"และ (3) การประยุกต์ใช้ในงานด้านต่างๆ เช่น Web Backend, DevOps Automation, " +
			"Cloud Computing และ IoT ตลอดจนวิเคราะห์จุดเด่น จุดด้อย " +
			"และความเหมาะสมของภาษา Go เมื่อเปรียบเทียบกับภาษาโปรแกรมยอดนิยมอื่น")
	w.bodyParagraph(
		"ผลการทบทวนเอกสารพบว่า Go มีประสิทธิภาพในการประมวลผลสูงใกล้เคียงภาษาระดับระบบ " +
			"มีโครงสร้างภาษาเรียบง่ายช่วยลดความซับซ้อนของโค้ด และมี ecosystem " +
			"ที่เติบโตอย่างต่อเนื่อง โดยเฉพาะในงานด้านระบบเครือข่ายและคลาวด์ " +
			"อย่างไรก็ตาม การที่เป็นภาษาค่อนข้างใหม่ส่งผลให้แหล่งข้อมูลและไลบรารีเฉพาะทาง" +
			"ยังไม่ครอบคลุมเท่าภาษาเดิมบางภาษา")

	// Keywords
	w.needsBreak(10)
	doc.SetFont("sarabun", "B", 11)
	doc.SetTextColor(0, 0, 0)
	w.page.SetX(w.lMargin)
	w.page.Cell(25, 6, "คำสำคัญ:", "", "L", false, 0)
	doc.SetFont("sarabun", "", 11)
	w.page.Cell(w.bodyWidth-25, 6, "ภาษา Go, Golang, Concurrency, ระบบซอฟต์แวร์สมัยใหม่, Backend", "", "L", false, 1)
	w.page.SetY(w.page.GetY() + 5)

	// ---- Section 1: Introduction ----
	w.sectionHeading("1. บทนำ")
	w.bodyParagraph(
		"การเติบโตของเทคโนโลยีดิจิทัลและระบบคลาวด์ส่งผลให้ระบบซอฟต์แวร์ต้องรองรับผู้ใช้งานจำนวนมาก " +
			"การประมวลผลแบบกระจาย และการทำงานพร้อมกันของบริการจำนวนมากภายใต้ข้อจำกัดด้านประสิทธิภาพ" +
			"และทรัพยากร การเลือกภาษาโปรแกรมและแพลตฟอร์มที่เหมาะสมจึงเป็นปัจจัยสำคัญต่อคุณภาพ" +
			"ของระบบซอฟต์แวร์ในภาพรวม ภาษาดั้งเดิมอย่าง C/C++ และ Java แม้มีประสิทธิภาพสูงแต่มี" +
			"ความซับซ้อนในการพัฒนาและบำรุงรักษา ในขณะที่ภาษาสคริปต์เช่น Python แม้พัฒนาได้รวดเร็ว" +
			"แต่มีข้อจำกัดด้านประสิทธิภาพเมื่อใช้งานในสเกลใหญ่")
	w.bodyParagraph(
		"ภาษาโปรแกรม Go หรือ Golang ถูกออกแบบขึ้นในปี ค.ศ. 2007 โดยทีมวิศวกรของ Google " +
			"เพื่อแก้ปัญหาข้างต้น และถูกเผยแพร่ในฐานะโอเพ่นซอร์สในปี ค.ศ. 2009 " +
			"โดยมีเป้าหมายผสมผสานความเร็วของภาษาแบบคอมไพล์ ความเรียบง่ายของไวยากรณ์ " +
			"และความสามารถในการรองรับงานพร้อมกันระดับสูง ปัจจุบันภาษา Go ได้ถูกนำไปใช้" +
			"ในโครงการสำคัญ เช่น Docker และ Kubernetes รวมถึงระบบ backend " +
			"และเครื่องมือด้าน DevOps จำนวนมาก")

	// ---- Section 2: Objectives ----
	w.sectionHeading("2. วัตถุประสงค์ของการศึกษา")
	w.bodyParagraph("1. เพื่ออธิบายแนวคิด หลักการออกแบบ และคุณลักษณะสำคัญของภาษาโปรแกรม Go (Golang)")
	w.bodyParagraph(
		"2. เพื่อวิเคราะห์ความเหมาะสมของภาษา Go ต่อการพัฒนาระบบซอฟต์แวร์สมัยใหม่" +
			"ในด้านประสิทธิภาพ ความง่ายในการพัฒนา และการรองรับงานพร้อมกัน")
	w.bodyParagraph(
		"3. เพื่อรวบรวมตัวอย่างการประยุกต์ใช้ภาษา Go ในงานด้าน Web Backend, " +
			"Cloud Computing, DevOps Automation และ IoT " +
			"พร้อมทั้งอภิปรายข้อดีข้อจำกัดของภาษาเมื่อเทียบกับภาษาอื่น")

	// ---- Section 3: Scope ----
	w.sectionHeading("3. ขอบเขตและวิธีดำเนินการศึกษา")
	w.bodyParagraph(
		"บทความฉบับนี้เป็นการวิจัยเชิงเอกสาร (documentary research) " +
			"โดยผู้วิจัยดำเนินการรวบรวมข้อมูลจากแหล่งข้อมูลออนไลน์ บทความวิชาการ " +
			"เอกสารสอน และเอกสารทางเทคนิคที่เผยแพร่สาธารณะเกี่ยวกับภาษา Go " +
			"ทั้งภาษาไทยและภาษาอังกฤษ จากนั้นจึงทำการวิเคราะห์ จัดหมวดหมู่ " +
			"และสังเคราะห์เนื้อหาตามประเด็นที่สอดคล้องกับวัตถุประสงค์การศึกษา")

	// ---- Section 4: Theory ----
	w.sectionHeading("4. แนวคิดและทฤษฎีที่เกี่ยวข้อง")

	w.subHeading("4.1 ภาษาโปรแกรมระดับระบบและภาษาโปรแกรมสมัยใหม่")
	w.bodyParagraph(
		"ภาษาโปรแกรมระดับระบบ (system programming languages) เช่น C และ C++ " +
			"เน้นการเข้าถึงหน่วยความจำระดับต่ำและประสิทธิภาพในการประมวลผล " +
			"แต่มีความซับซ้อนสูง การจัดการหน่วยความจำเอง " +
			"และความเสี่ยงต่อข้อผิดพลาด เช่น memory leak หรือ pointer error " +
			"ในทางตรงกันข้าม ภาษาโปรแกรมสมัยใหม่ เช่น Java, C#, Python และ JavaScript " +
			"เน้น productivity ของนักพัฒนา มีการจัดการหน่วยความจำอัตโนมัติและ ecosystem ขนาดใหญ่ " +
			"แต่บางครั้งต้องแลกมาด้วยต้นทุนด้านประสิทธิภาพ")
	w.bodyParagraph(
		"ภาษา Go อยู่กึ่งกลางระหว่างสองขั้วนี้ โดยเป็นภาษาแบบคอมไพล์และ static typing " +
			"ที่ยังคงประสิทธิภาพระดับสูง แต่ลดความซับซ้อนของไวยากรณ์และการจัดการหน่วยความจำ " +
			"ผ่าน garbage collector และมาตรฐานของแพ็กเกจไลบรารี")

	w.subHeading("4.2 แนวคิดการทำงานพร้อมกัน (Concurrency)")
	w.bodyParagraph(
		"Concurrency เป็นแนวคิดสำคัญของระบบซอฟต์แวร์สมัยใหม่ที่ต้องรองรับการประมวลผล" +
			"หลายงานในเวลาเดียวกัน ภาษา Go นำเสนอโมเดล concurrency ผ่าน goroutines " +
			"ซึ่งเป็นหน่วยการทำงานขนาดเบาที่รันฟังก์ชันพร้อมกัน และ channel " +
			"ซึ่งเป็นกลไกส่งผ่านข้อมูลระหว่าง goroutines ตามหลักการ " +
			"\"do not communicate by sharing memory; share memory by communicating\" " +
			"แนวคิดนี้ช่วยลดความยุ่งยากของการใช้ threads และ lock แบบดั้งเดิม " +
			"และเป็นเหตุผลหนึ่งที่ทำให้ Go เหมาะกับงานด้านระบบเครือข่ายและบริการแบบกระจาย")

	// ---- Section 5: Go language ----
	w.sectionHeading("5. ภาษาโปรแกรม Go (Golang)")

	w.subHeading("5.1 ประวัติและหลักการออกแบบ")
	w.bodyParagraph(
		"ภาษา Go ถูกออกแบบโดย Robert Griesemer, Rob Pike และ Ken Thompson " +
			"วิศวกรของ Google ที่มีประสบการณ์ด้านระบบปฏิบัติการและภาษาคอมพิวเตอร์มายาวนาน " +
			"โดยเริ่มโครงการในปี ค.ศ. 2007 และเปิดเผยสู่สาธารณะในปี ค.ศ. 2009 " +
			"หลักการสำคัญของการออกแบบภาษา Go คือความเรียบง่าย (simplicity) " +
			"ความชัดเจน (clarity) และประสิทธิภาพ (efficiency)")

	w.subHeading("5.2 คุณลักษณะเชิงเทคนิค")
	w.bodyParagraph(
		"1. เป็นภาษา static type compiled language: " +
			"ตัวแปรต้องระบุชนิดข้อมูลชัดเจนและคอมไพล์เป็นโค้ดเครื่อง " +
			"ทำให้ตรวจจับข้อผิดพลาดด้านชนิดข้อมูลได้ตั้งแต่ขั้นตอนคอมไพล์")
	w.bodyParagraph(
		"2. มีเวลาคอมไพล์รวดเร็ว: " +
			"คอมไพเลอร์ของ Go ถูกออกแบบมาให้ build โครงการขนาดใหญ่ได้อย่างรวดเร็ว")
	w.bodyParagraph(
		"3. รองรับ concurrency ในตัวภาษา: ผ่าน goroutines และ channels " +
			"ทำให้การเขียนโปรแกรมที่ต้องการงานพร้อมกันจำนวนมากทำได้สะดวก")
	w.bodyParagraph(
		"4. มี standard library ครอบคลุม: " +
			"รวมทั้งแพ็กเกจสำหรับงานเครือข่าย การเข้ารหัส การจัดการไฟล์ " +
			"การทดสอบ และการเขียนเว็บเซิร์ฟเวอร์")
	w.bodyParagraph(
		"5. รองรับการทดสอบในตัวภาษา: Go มีโครงสร้างการเขียน unit test " +
			"และ benchmark ในชุดเครื่องมือมาตรฐาน")

	w.subHeading("5.3 ความเรียบง่ายของไวยากรณ์")
	w.bodyParagraph(
		"ไวยากรณ์ของ Go ได้รับการออกแบบให้ลดรูป เช่น ไม่มีการสืบทอดแบบคลาสหลายระดับ " +
			"ไม่มีการโอเวอร์โหลดฟังก์ชัน และใช้การจัดรูปแบบโค้ดด้วย gofmt " +
			"ตามมาตรฐานเดียวกันทั้งโครงการ ทำให้โค้ดมีลักษณะสม่ำเสมอและอ่านง่าย " +
			"แนวทางดังกล่าวช่วยลดการถกเถียงเรื่องสไตล์" +
			"และเปิดโอกาสให้นักพัฒนามุ่งเน้นไปที่ตรรกะของระบบมากกว่ารูปแบบการเขียนโค้ด")

	// ---- Section 6: Applications ----
	w.sectionHeading("6. การประยุกต์ใช้ภาษา Go ในระบบซอฟต์แวร์สมัยใหม่")

	w.subHeading("6.1 งานด้าน Web Backend และ API")
	w.bodyParagraph(
		"ภาษา Go ได้รับความนิยมสูงในการพัฒนา Web Backend และ RESTful API " +
			"เนื่องจากประสิทธิภาพในการรองรับคำร้อง (request) จำนวนมาก " +
			"ความสามารถในการทำ concurrency และแพ็กเกจ net/http ใน standard library " +
			"ที่ใช้งานได้ทันทีโดยไม่ต้องติดตั้งเฟรมเวิร์กเพิ่มเติม " +
			"นอกจากนี้ยังมีเฟรมเวิร์กยอดนิยม เช่น Gin, Echo และ Fiber " +
			"ที่ช่วยให้พัฒนา API ได้รวดเร็วขึ้น")

	w.subHeading("6.2 DevOps Automation และเครื่องมือระบบ")
	w.bodyParagraph(
		"ด้วย binary ที่คอมไพล์ได้เป็นไฟล์เดียว น้ำหนักเบา และรันได้บนหลายแพลตฟอร์ม " +
			"ทำให้ Go เหมาะอย่างยิ่งสำหรับการพัฒนาเครื่องมือบรรทัดคำสั่ง (CLI tools) " +
			"และสคริปต์อัตโนมัติในงาน DevOps โครงการสำคัญอย่าง Docker และ Kubernetes " +
			"ใช้ภาษา Go เป็นหลัก สะท้อนให้เห็นถึงศักยภาพของภาษาในการสร้างเครื่องมือ" +
			"โครงสร้างพื้นฐานระดับโลก")

	w.subHeading("6.3 Cloud Computing และ Distributed Systems")
	w.bodyParagraph(
		"ภาษา Go ถูกนำไปใช้ในระบบคลาวด์และระบบแบบกระจาย (distributed systems) " +
			"อย่างแพร่หลาย เนื่องจากความสามารถด้าน concurrency " +
			"และการจัดการเครือข่ายที่มีประสิทธิภาพ " +
			"ตัวอย่างเช่น ระบบของบริษัท Dropbox มีการย้ายส่วน backend จำนวนมาก" +
			"จากภาษาอื่นมาใช้ Go เพื่อเพิ่มประสิทธิภาพและลดความซับซ้อนของโค้ด")

	w.subHeading("6.4 การประยุกต์ใช้ในงาน IoT")
	w.bodyParagraph(
		"งานวิจัยและบทความด้าน IoT พบว่าภาษา Go มีความเหมาะสมในการพัฒนา" +
			"แอปพลิเคชันสำหรับอุปกรณ์ IoT ที่ต้องการความเร็วและความเสถียร " +
			"เนื่องจากสามารถสร้างโปรแกรมที่มี footprint เล็ก" +
			"และรองรับการเชื่อมต่อเครือข่ายจำนวนมากได้ดี")

	// ---- Section 7: Comparison table ----
	w.sectionHeading("7. การเปรียบเทียบข้อดีข้อจำกัดของภาษา Go")
	w.needsBreak(45) // table: header + 4 rows at 7mm each + padding
	w.drawComparisonTable()
	w.page.SetY(w.page.GetY() + 3)
	w.bodyParagraph(
		"ข้อดีสำคัญของ Go คือประสิทธิภาพสูง การสนับสนุน concurrency ในระดับภาษา " +
			"และความเรียบง่ายของโค้ด ซึ่งช่วยให้เหมาะกับระบบที่ต้องสเกล" +
			"และต้องการเสถียรภาพในระยะยาว อย่างไรก็ตาม ข้อจำกัด ได้แก่ " +
			"ความใหม่ของภาษาเมื่อเทียบกับภาษาอื่น ทำให้บางสาขาอาจยังมีไลบรารี" +
			"และตัวอย่างไม่มาก รวมถึงการไม่มีฟีเจอร์บางอย่าง เช่น การโอเวอร์โหลดฟังก์ชัน")

	// ---- Section 8: Discussion ----
	w.sectionHeading("8. อภิปรายผล")
	w.bodyParagraph(
		"จากการทบทวนเอกสาร พบว่าภาษา Go ถือเป็นทางเลือกที่น่าสนใจอย่างยิ่ง" +
			"สำหรับระบบซอฟต์แวร์สมัยใหม่ที่ต้องการทั้งประสิทธิภาพและความเรียบง่าย" +
			"ในการพัฒนา โดยเฉพาะในบริบทของบริการบนคลาวด์ ระบบไมโครเซอร์วิส " +
			"และเครื่องมือด้าน DevOps ที่ต้องรองรับงานพร้อมกันจำนวนมาก " +
			"การออกแบบให้ concurrency เป็นความสามารถระดับภาษา" +
			"และการมี standard library ที่ครอบคลุม" +
			"ช่วยลดภาระการเลือกใช้เฟรมเวิร์กภายนอกและลดความซับซ้อนของระบบในภาพรวม")
	w.bodyParagraph(
		"ในทางวิชาการ ภาษา Go เป็นกรณีศึกษาที่น่าสนใจของการออกแบบภาษาโปรแกรม" +
			"ที่ให้ความสำคัญกับ productivity และ maintainability " +
			"ของบุคลากรด้านซอฟต์แวร์ในองค์กรขนาดใหญ่ " +
			"ขณะเดียวกันก็ไม่ละเลยประสิทธิภาพเชิงเทคนิค " +
			"อย่างไรก็ตาม การตัดสินใจเลือกใช้ภาษาใดในโครงการหนึ่งๆ " +
			"ยังต้องพิจารณาปัจจัยอื่น เช่น ความพร้อมของทีม " +
			"พื้นฐานเดิมของนักพัฒนา และ ecosystem ในโดเมนงานเฉพาะ")

	// ---- Section 9: Conclusion ----
	w.sectionHeading("9. สรุปและข้อเสนอแนะ")
	w.bodyParagraph(
		"บทความวิชาการฉบับนี้ได้สังเคราะห์ข้อมูลเกี่ยวกับภาษาโปรแกรม Go (Golang) " +
			"ในมิติของประวัติ หลักการออกแบบ คุณลักษณะเชิงเทคนิค " +
			"และการประยุกต์ใช้ในงานซอฟต์แวร์สมัยใหม่ " +
			"ผลการศึกษาแสดงให้เห็นว่า Go มีศักยภาพสูงในงานด้าน " +
			"Web Backend, DevOps Automation, Cloud Computing และ IoT " +
			"เนื่องจากประสิทธิภาพและความสามารถด้าน concurrency ที่โดดเด่น")
	w.bodyParagraph(
		"ข้อเสนอแนะเชิงวิชาชีพ คือ องค์กรที่กำลังพัฒนาระบบใหม่" +
			"โดยเฉพาะระบบที่มีสถาปัตยกรรมแบบไมโครเซอร์วิส" +
			"หรือมีภาระงานด้านเครือข่ายจำนวนมาก " +
			"ควรพิจารณาภาษา Go เป็นหนึ่งในทางเลือกหลัก " +
			"โดยให้ความสำคัญกับการเตรียมความพร้อมของทีม" +
			"และการจัดทำแนวทางการเขียนโค้ด (coding guideline) " +
			"ให้สอดคล้องกับแนวคิดของภาษา")

	// ---- Section 10: References ----
	w.sectionHeading("10. เอกสารอ้างอิง")

	refs := []string{
		"Skooldio. (2564). Golang คืออะไร? ดียังไง? รวมสิ่งที่ควรรู้เกี่ยวกับ Golang.",
		"LINE TODAY. (2568). Golang หรือ ภาษา Go คืออะไร? รู้จักภาษาโปรแกรมยุคใหม่จาก Google.",
		"TAmemo. (2563). Golang 101: ทำความรู้จักภาษาโกฉบับโปรแกรมเมอร์.",
		"Skooldio. รู้จักกับภาษา Go ภาษาที่สร้างโดย Google.",
		"Thaiware. (2568). Golang หรือ ภาษา Go คืออะไร?",
		"PALO IT. Golang for Beginners.",
		"The Gang. รู้จัก Golang ภาษาที่เหมาะกับ Backend.",
		"Deeploytech. การใช้ Go ในการพัฒนา IoT Applications.",
		"Wikipedia. Go (programming language).",
	}

	doc.SetFont("sarabun", "", 10)
	doc.SetTextColor(0, 0, 0)
	for _, ref := range refs {
		w.needsBreak(7)
		w.page.SetX(w.lMargin)
		w.page.MultiCell(w.bodyWidth, 5, "- "+ref, "", "L", false)
		w.page.SetY(w.page.GetY() + 1)
	}

	// ---- Save ----
	path := "/tmp/folio_thai_article.pdf"
	if err := doc.Save(path); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Thai article PDF saved to %s\n", path)
}

// --- Helper methods on writer ---

func (w *writer) sectionHeading(text string) {
	w.needsBreak(15)
	w.doc.SetFont("sarabun", "B", 14)
	w.doc.SetTextColor(0, 51, 102)
	w.page.SetX(w.lMargin)
	w.page.Cell(w.bodyWidth, 8, text, "", "L", false, 1)
	w.page.SetY(w.page.GetY() + 1)
}

func (w *writer) subHeading(text string) {
	w.needsBreak(12)
	w.doc.SetFont("sarabun", "B", 12)
	w.doc.SetTextColor(40, 80, 140)
	w.page.SetX(w.lMargin)
	w.page.Cell(w.bodyWidth, 7, text, "", "L", false, 1)
	w.page.SetY(w.page.GetY() + 1)
}

func (w *writer) bodyParagraph(text string) {
	w.doc.SetFont("sarabun", "", 11)
	w.doc.SetTextColor(0, 0, 0)

	lineH := 6.0
	estH := w.estimateHeight(text, lineH)
	w.needsBreak(estH + 2)

	w.page.SetX(w.lMargin)
	w.page.MultiCell(w.bodyWidth, lineH, text, "", "J", false)
	w.page.SetY(w.page.GetY() + 2)
}

// estimateHeight estimates the height of a MultiCell by counting how many lines it will produce.
func (w *writer) estimateHeight(text string, lineH float64) float64 {
	sw := w.page.GetStringWidth(text)
	availW := w.bodyWidth - 4 // subtract cell margins (2mm each side)
	lines := sw / availW
	if lines < 1 {
		lines = 1
	}
	// Add 1 extra line for word-wrap overhead (words don't break mid-word)
	return (lines + 1) * lineH
}

func (w *writer) drawComparisonTable() {
	colW := []float64{35, 65, 65}
	x0 := w.lMargin
	rowH := 7.0

	// Header
	w.doc.SetFont("sarabun", "B", 10)
	w.doc.SetFillColor(0, 51, 102)
	w.doc.SetTextColor(255, 255, 255)

	headers := []string{"ประเด็น", "ภาษา Go (Golang)", "ภาษา Python"}
	w.page.SetX(x0)
	for i, h := range headers {
		w.page.Cell(colW[i], rowH, h, "1", "C", true, 0)
	}
	w.page.SetXY(x0, w.page.GetY()+rowH)

	type row struct {
		label, go_, py string
	}
	rows := []row{
		{"ประสิทธิภาพ", "สูง ใกล้เคียงภาษาคอมไพล์", "ต่ำกว่า หากไม่ใช้ native"},
		{"ความง่าย", "ไวยากรณ์เรียบง่าย ชัดเจน", "เขียนสั้น ยืดหยุ่นมาก"},
		{"Concurrency", "goroutines, channels", "ต้องใช้ threading/async"},
		{"Ecosystem", "เติบโตเร็ว เน้น backend", "ใหญ่มาก หลากหลายสาขา"},
	}

	w.doc.SetFont("sarabun", "", 10)
	for i, r := range rows {
		if i%2 == 0 {
			w.doc.SetFillColor(235, 241, 250)
		} else {
			w.doc.SetFillColor(255, 255, 255)
		}
		w.doc.SetTextColor(0, 0, 0)

		w.page.SetX(x0)
		w.doc.SetFont("sarabun", "B", 10)
		w.page.Cell(colW[0], rowH, r.label, "1", "L", true, 0)
		w.doc.SetFont("sarabun", "", 10)
		w.page.Cell(colW[1], rowH, r.go_, "1", "L", true, 0)
		w.page.Cell(colW[2], rowH, r.py, "1", "L", true, 0)
		w.page.SetXY(x0, w.page.GetY()+rowH)
	}
}
